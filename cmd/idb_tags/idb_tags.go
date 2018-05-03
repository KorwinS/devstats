package main

import (
	"strings"
	"time"

	lib "devstats"

	yaml "gopkg.in/yaml.v2"
)

// tags contain list of InfluxDB tags
type tags struct {
	Tags []tag `yaml:"tags"`
}

// tag contain each InfluxDB tag data
type tag struct {
	Name       string `yaml:"name"`
	SQLFile    string `yaml:"sql"`
	SeriesName string `yaml:"series_name"`
	NameTag    string `yaml:"name_tag"`
	ValueTag   string `yaml:"value_tag"`
}

// Insert InfluxDB tags
func idbTags() {
	// Environment context parse
	var ctx lib.Ctx
	ctx.Init()

	// Connect to Postgres DB
	con := lib.PgConn(&ctx)
	defer func() { lib.FatalOnError(con.Close()) }()

	// Local or cron mode?
	dataPrefix := lib.DataDir
	if ctx.Local {
		dataPrefix = "./"
	}

	// Read tags to generate
	data, err := lib.ReadFile(&ctx, dataPrefix+ctx.TagsYaml)
	if err != nil {
		lib.FatalOnError(err)
		return
	}
	var allTags tags
	lib.FatalOnError(yaml.Unmarshal(data, &allTags))

	// Per project directory for SQL files
	dir := lib.Metrics
	if ctx.Project != "" {
		dir += ctx.Project + "/"
	}

	thrN := lib.GetThreadsNum(&ctx)
	// Iterate tags
	ch := make(chan bool)
	nThreads := 0
	// Use integer index to pass to go rountine
	for i := range allTags.Tags {
		go func(ch chan bool, idx int) {
			// Refer to current tag using index passed to anonymous function
			tg := &allTags.Tags[idx]
			if ctx.Debug > 0 {
				lib.Printf("Tag '%s' --> '%s'\n", tg.Name, tg.SeriesName)
			}

			// Get BatchPoints
			var pts lib.TSPoints

			// Read SQL file
			bytes, err := lib.ReadFile(&ctx, dataPrefix+dir+tg.SQLFile+".sql")
			lib.FatalOnError(err)
			sqlQuery := string(bytes)

			// Handle excluding bots
			bytes, err = lib.ReadFile(&ctx, dataPrefix+"util_sql/exclude_bots.sql")
			lib.FatalOnError(err)
			excludeBots := string(bytes)

			// Transform SQL
			sqlQuery = strings.Replace(sqlQuery, "{{lim}}", "69", -1)
			sqlQuery = strings.Replace(sqlQuery, "{{exclude_bots}}", excludeBots, -1)

			// Execute SQL
			rows := lib.QuerySQLWithErr(con, &ctx, sqlQuery)
			defer func() { lib.FatalOnError(rows.Close()) }()

			// Drop current tags
			table := "t" + tg.SeriesName
			if lib.TableExists(con, &ctx, table) {
				lib.ExecSQLWithErr(con, &ctx, "truncate "+table)
			}
			tm := lib.TimeParseAny("2014-01-01")

			// Iterate tag values
			tags := make(map[string]string)
			strVal := ""
			for rows.Next() {
				lib.FatalOnError(rows.Scan(&strVal))
				if ctx.Debug > 0 {
					lib.Printf("'%s': %v\n", tg.SeriesName, strVal)
				}
				if tg.NameTag != "" {
					tags[tg.NameTag] = strVal
				}
				if tg.ValueTag != "" {
					tags[tg.ValueTag] = lib.NormalizeName(strVal)
				}
				// Add batch point
				pt := lib.NewTSPoint(&ctx, tg.SeriesName, tags, nil, tm)
				lib.AddTSPoint(&ctx, &pts, pt)
				tm = tm.Add(time.Hour)
			}
			lib.FatalOnError(rows.Err())

			// Write the batch
			if !ctx.SkipIDB {
				lib.WriteTSPoints(&ctx, con, &pts, nil)
			} else if ctx.Debug > 0 {
				lib.Printf("Skipping tags series write\n")
			}

			// Synchronize go routine
			if ch != nil {
				ch <- true
			}
		}(ch, i)
		// go routine called with 'ch' channel to sync and tag index
		nThreads++
		if nThreads == thrN {
			<-ch
			nThreads--
		}
	}
	// Usually all work happens on '<-ch'
	lib.Printf("Final threads join\n")
	for nThreads > 0 {
		<-ch
		nThreads--
	}
}

func main() {
	dtStart := time.Now()
	idbTags()
	dtEnd := time.Now()
	lib.Printf("Time: %v\n", dtEnd.Sub(dtStart))
}
