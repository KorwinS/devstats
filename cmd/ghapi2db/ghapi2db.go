package main

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	lib "devstats"

	"github.com/google/go-github/github"
)

// deleteArtificialEvent - deletes artificial event from all tables
func deleteArtificialEvent(c *sql.DB, ctx *lib.Ctx, eid int64) (err error) {
	if ctx.SkipPDB {
		if ctx.Debug > 0 {
			lib.Printf("Skipping delete artificial event: %d\n", eid)
		}
		return nil
	}

	// Start transaction
	tc, err := c.Begin()
	lib.FatalOnError(err)

	// Delete from gha_events, gha_issues, gha_payloads, gha_issues_labels
	lib.ExecSQLTxWithErr(tc, ctx, fmt.Sprintf("delete from gha_events where id = %s", lib.NValue(1)), eid)
	lib.ExecSQLTxWithErr(tc, ctx, fmt.Sprintf("delete from gha_issues where event_id = %s", lib.NValue(1)), eid)
	lib.ExecSQLTxWithErr(tc, ctx, fmt.Sprintf("delete from gha_payloads where event_id = %s", lib.NValue(1)), eid)
	lib.ExecSQLTxWithErr(tc, ctx, fmt.Sprintf("delete from gha_issues_labels where event_id = %s", lib.NValue(1)), eid)

	// Final commit
	lib.FatalOnError(tc.Commit())
	//lib.FatalOnError(tc.Rollback())
	return
}

// cleanArtificialEvents sometimes we're adding artificial event when syncing at hh:8
// while somebody else already added new comment that made up-to-date milestone and/or label list available to GHA
// but we will have that data next hour and unneeded artificial event is created.
// Function detects such events from past, and remove them.
// Event is marked as "not needed" when previous or next event on the same issue has the same milestone and label set.
func cleanArtificialEvents(ctx *lib.Ctx) {
	// Connect to Postgres DB
	c := lib.PgConn(ctx)
	defer func() { lib.FatalOnError(c.Close()) }()

	// Get number of CPUs available
	thrN := lib.GetThreadsNum(ctx)
	lib.Printf("ghapi2db.go: Running cleanup artificial events (on %d CPUs)\n", thrN)

	var rows *sql.Rows
	minEventID := 281474976710656
	if len(ctx.OnlyEvents) > 0 {
		ary := []string{}
		for _, event := range ctx.OnlyEvents {
			ary = append(ary, strconv.FormatInt(event, 10))
		}
		lib.Printf("Processing only selected %d %v events for debugging\n", len(ctx.OnlyEvents), ctx.OnlyEvents)
		rows = lib.QuerySQLWithErr(
			c,
			ctx,
			fmt.Sprintf(
				"select id, event_id, milestone_id, updated_at from gha_issues where "+
					"event_id > %d and event_id in (%s)",
				minEventID,
				strings.Join(ary, ","),
			),
		)
	} else {
		// Get all artificial events in the recent range
		rows = lib.QuerySQLWithErr(
			c,
			ctx,
			fmt.Sprintf(
				"select id, event_id, milestone_id, updated_at from gha_issues "+
					"where event_id > %s and updated_at > now() - %s::interval",
				lib.NValue(1),
				lib.NValue(2),
			),
			minEventID,
			ctx.RecentRange,
		)
	}
	defer func() { lib.FatalOnError(rows.Close()) }()
	var (
		issueID     int64
		eventID     int64
		milestoneID *int64
		updatedAt   time.Time
	)
	ch := make(chan bool)
	nThreads := 0
	nRows := 0
	var counterMutex = &sync.Mutex{}
	deleted := 0
	for rows.Next() {
		lib.FatalOnError(rows.Scan(&issueID, &eventID, &milestoneID, &updatedAt))
		go func(ch chan bool, iid int64, eid int64, mid *int64, updated time.Time) {
			// Synchronize go routine
			defer func(c chan bool) { c <- true }(ch)

			// Get last event before this artificial event
			rowsP := lib.QuerySQLWithErr(
				c,
				ctx,
				fmt.Sprintf(
					"select event_id, milestone_id, updated_at from gha_issues "+
						"where id = %s and updated_at < %s order by updated_at desc limit 1",
					lib.NValue(1),
					lib.NValue(2),
				),
				iid,
				updated,
			)
			defer func() { lib.FatalOnError(rowsP.Close()) }()
			var (
				peid     int64
				pmid     *int64
				pupdated time.Time
			)
			for rowsP.Next() {
				lib.FatalOnError(rowsP.Scan(&peid, &pmid, &pupdated))
			}
			lib.FatalOnError(rowsP.Err())

			// Check if they differ by milestone, if so, then we are done
			smid := lib.Null
			spmid := lib.Null
			if mid != nil {
				smid = strconv.FormatInt(*mid, 10)
			}
			if pmid != nil {
				spmid = strconv.FormatInt(*pmid, 10)
			}
			if smid != spmid {
				if ctx.Debug > 0 {
					lib.Printf(
						"Issue %d, Event (%d, %d), Dates(%v, %v): milestone difference artificial: %s != previus: %s\n",
						iid, eid, peid, lib.ToYMDHMSDate(updated), lib.ToYMDHMSDate(pupdated), smid, spmid,
					)
				}
				return
			}

			// Process current labels
			rowsL := lib.QuerySQLWithErr(
				c,
				ctx,
				fmt.Sprintf(
					"select coalesce(string_agg(sub.label_id::text, ','), '') from "+
						"(select label_id from gha_issues_labels where event_id = %s "+
						"order by label_id) sub",
					lib.NValue(1),
				),
				eid,
			)
			defer func() { lib.FatalOnError(rowsL.Close()) }()
			labels := ""
			for rowsL.Next() {
				lib.FatalOnError(rowsL.Scan(&labels))
			}
			lib.FatalOnError(rowsL.Err())

			// Process previous labels
			rowsLP := lib.QuerySQLWithErr(
				c,
				ctx,
				fmt.Sprintf(
					"select coalesce(string_agg(sub.label_id::text, ','), '') from "+
						"(select label_id from gha_issues_labels where event_id = %s "+
						"order by label_id) sub",
					lib.NValue(1),
				),
				peid,
			)
			defer func() { lib.FatalOnError(rowsLP.Close()) }()
			plabels := ""
			for rowsLP.Next() {
				lib.FatalOnError(rowsLP.Scan(&plabels))
			}
			lib.FatalOnError(rowsLP.Err())

			// Check if they differ by labels, if so, then we are done
			if labels != plabels {
				if ctx.Debug > 0 {
					lib.Printf(
						"Issue %d, Event (%d, %d), Dates(%v, %v): label set difference artificial: '%s' != previus: '%s'\n",
						iid, eid, peid, lib.ToYMDHMSDate(updated), lib.ToYMDHMSDate(pupdated), labels, plabels,
					)
				}
				return
			}

			// Get first event after this artificial event, not newer than 2 hours
			updated2h := lib.HourStart(updated).Add(2 * time.Hour)
			rowsN := lib.QuerySQLWithErr(
				c,
				ctx,
				fmt.Sprintf(
					"select event_id, milestone_id, updated_at from gha_issues "+
						"where id = %s and updated_at > %s and updated_at < %s "+
						"order by updated_at asc limit 1",
					lib.NValue(1),
					lib.NValue(2),
					lib.NValue(3),
				),
				iid,
				updated,
				updated2h,
			)
			defer func() { lib.FatalOnError(rowsN.Close()) }()
			var (
				neid     int64
				nmid     *int64
				nupdated time.Time
			)
			ngot := false
			for rowsN.Next() {
				lib.FatalOnError(rowsN.Scan(&neid, &nmid, &nupdated))
				ngot = true
			}
			lib.FatalOnError(rowsN.Err())

			// If there is no new event yet, keep artificial event
			if !ngot {
				if ctx.Debug > 0 {
					lib.Printf(
						"Issue %d, Event (%d, -), Dates(%v, -): there is no next event in the next 2 hours\n",
						iid, eid, lib.ToYMDHMSDate(updated),
					)
				}
				return
			}

			// Check if they differ by milestone, if so, then we are done
			snmid := lib.Null
			if nmid != nil {
				snmid = strconv.FormatInt(*nmid, 10)
			}
			if smid != snmid {
				if ctx.Debug > 0 {
					lib.Printf(
						"Issue %d, Event (%d, %d), Dates(%v, %v): milestone difference artificial: %s != next: %s\n",
						iid, eid, neid, lib.ToYMDHMSDate(updated), lib.ToYMDHMSDate(nupdated), smid, snmid,
					)
				}
				return
			}

			// Process previous labels
			rowsLN := lib.QuerySQLWithErr(
				c,
				ctx,
				fmt.Sprintf(
					"select coalesce(string_agg(sub.label_id::text, ','), '') from "+
						"(select label_id from gha_issues_labels where event_id = %s "+
						"order by label_id) sub",
					lib.NValue(1),
				),
				neid,
			)
			defer func() { lib.FatalOnError(rowsLN.Close()) }()
			nlabels := ""
			for rowsLN.Next() {
				lib.FatalOnError(rowsLN.Scan(&nlabels))
			}
			lib.FatalOnError(rowsLN.Err())

			// Check if they differ by labels, if so, then we are done
			if labels != nlabels {
				if ctx.Debug > 0 {
					lib.Printf(
						"Issue %d, Event (%d, %d), Dates(%v, %v): label set difference artificial: '%s' != next: '%s'\n",
						iid, eid, neid, lib.ToYMDHMSDate(updated), lib.ToYMDHMSDate(nupdated), labels, nlabels,
					)
				}
				return
			}

			// Now we can delete this event
			if ctx.Debug > 0 {
				lib.Printf(
					"Removing artificial event:\n"+
						"iid=%d  eid=%d  mid=%v  labels=%s  updated=%v\n"+
						"iid=%d peid=%d      pmid=%v plabels=%s pupdated=%v\n"+
						"iid=%d neid=%d      nmid=%v nlabels=%s nupdated=%v\n\n",
					iid, eid, mid, labels, lib.ToYMDHMSDate(updated),
					iid, peid, pmid, plabels, lib.ToYMDHMSDate(pupdated),
					iid, neid, nmid, nlabels, lib.ToYMDHMSDate(nupdated),
				)
			}
			// Delete artificial event
			lib.FatalOnError(deleteArtificialEvent(c, ctx, eid))

			// Safe increase counter
			counterMutex.Lock()
			deleted++
			counterMutex.Unlock()
		}(ch, issueID, eventID, milestoneID, updatedAt)

		nThreads++
		if nThreads == thrN {
			<-ch
			nRows++
			nThreads--
		}
	}
	// Usually all work happens on '<-ch'
	lib.Printf("Final artificial events clean threads join\n")
	for nThreads > 0 {
		<-ch
		nRows++
		nThreads--
	}
	lib.FatalOnError(rows.Err())
	lib.Printf("Processed %d artificial events, deleted %d\n", nRows, deleted)
}

// Insert Postgres vars
func ghapi2db(ctx *lib.Ctx) {
	// Connect to Postgres DB
	c := lib.PgConn(ctx)
	defer func() { lib.FatalOnError(c.Close()) }()

	// Connect to GitHub API
	gctx, gc := lib.GHClient(ctx)

	// Get RateLimits info
	_, rem, wait := lib.GetRateLimits(gctx, gc, true)

	// Get number of CPUs available
	thrN := lib.GetThreadsNum(ctx)
	lib.Printf("ghapi2db.go: Running (on %d CPUs): %d API points available, resets in %v\n", thrN, rem, wait)

	// Local or cron mode?
	dataPrefix := lib.DataDir
	if ctx.Local {
		dataPrefix = "./"
	}
	// Get recently modified opened issues/PRs
	bytes, err := lib.ReadFile(
		ctx,
		dataPrefix+"util_sql/open_issues_and_prs.sql",
	)
	lib.FatalOnError(err)
	sqlQuery := string(bytes)

	// Set range from a context
	sqlQuery = strings.Replace(sqlQuery, "{{period}}", ctx.RecentRange, -1)
	rows := lib.QuerySQLWithErr(c, ctx, sqlQuery)
	defer func() { lib.FatalOnError(rows.Close()) }()

	// Get issues/PRs to check
	// repo, number, issueID, is_pr
	var issuesMutex = &sync.RWMutex{}
	issues := make(map[int64]lib.IssueConfigAry)
	var (
		repo    string
		number  int
		issueID int64
		pr      bool
	)
	nIssues := 0
	for rows.Next() {
		lib.FatalOnError(rows.Scan(&repo, &number, &issueID, &pr))
		cfg := lib.IssueConfig{
			Repo:    repo,
			Number:  number,
			IssueID: issueID,
			Pr:      pr,
		}
		v, ok := issues[issueID]
		if ok {
			if ctx.Debug > 0 {
				lib.Printf("Warning: we already have issue config for id=%d: %v, skipped new config: %v\n", issueID, v[0], cfg)
			}
			continue
		}
		issues[issueID] = lib.IssueConfigAry{cfg}
		nIssues++
		if ctx.Debug > 0 {
			lib.Printf("Open Issue ID '%d' --> '%v'\n", issueID, cfg)
		}
	}
	lib.FatalOnError(rows.Err())
	if ctx.Debug > 0 {
		lib.Printf("Got %d open issues for period %s\n", nIssues, ctx.RecentRange)
	}

	if len(ctx.OnlyIssues) > 0 {
		ary := []string{}
		for _, issue := range ctx.OnlyIssues {
			ary = append(ary, strconv.FormatInt(issue, 10))
		}
		onlyIssues := make(map[int64]lib.IssueConfigAry)
		nOnlyIssues := 0
		lib.Printf("Processing only selected %d %v issues for debugging\n", len(ctx.OnlyIssues), ctx.OnlyIssues)
		irows := lib.QuerySQLWithErr(
			c,
			ctx,
			fmt.Sprintf(
				"select distinct dup_repo_name, number, id, is_pull_request from gha_issues where id in (%s)",
				strings.Join(ary, ","),
			),
		)
		defer func() { lib.FatalOnError(irows.Close()) }()
		for irows.Next() {
			lib.FatalOnError(irows.Scan(&repo, &number, &issueID, &pr))
			cfg := lib.IssueConfig{
				Repo:    repo,
				Number:  number,
				IssueID: issueID,
				Pr:      pr,
			}
			v, ok := onlyIssues[issueID]
			if ok {
				if ctx.Debug > 0 {
					lib.Printf("Warning: we already have issue config for id=%d: %v, skipped new config: %v\n", issueID, v[0], cfg)
				}
				continue
			}
			onlyIssues[issueID] = lib.IssueConfigAry{cfg}
			nOnlyIssues++
			_, ok = issues[issueID]
			if ok {
				lib.Printf("Issue %d(%v) would also be processed by the default workflow\n", issueID, cfg)
			} else {
				lib.Printf("Issue %d(%v) would not be processed by the default workflow\n", issueID, cfg)
			}
		}
		lib.FatalOnError(irows.Err())
		lib.Printf("Processing %d/%d user provided issues\n", nOnlyIssues, len(ctx.OnlyIssues))
		issues = onlyIssues
		nIssues = nOnlyIssues
	}

	// GitHub paging config
	opt := &github.ListOptions{PerPage: 1000}
	// GitHub don't like MT quering - they say that:
	// 403 You have triggered an abuse detection mechanism. Please wait a few minutes before you try again
	// So let's get all GitHub stuff one-after-another (ugly and slow) and then spawn threads to speedup
	// Damn GitHub! - this could be working Number of CPU times faster! We're trying some hardcoded value: allowedThrN
	// Seems like GitHub is not detecting abuse when using 16 thread, but it detects when using 32.
	allowedThrN := 16
	if allowedThrN > thrN {
		allowedThrN = thrN
	}
	ch := make(chan bool)
	nThreads := 0
	dtStart := time.Now()
	lastTime := dtStart
	checked := 0
	lib.Printf("ghapi2db.go: Processing %d issues - GHAPI part\n", nIssues)
	// Create keys array to avoid accessing shared issues map concurently
	keys := []int64{}
	for key := range issues {
		keys = append(keys, key)
	}
	for _, key := range keys {
		go func(ch chan bool, iid int64) {
			// Refer to current tag using index passed to anonymous function
			issuesMutex.RLock()
			cfg := issues[iid][0]
			issuesMutex.RUnlock()
			if ctx.Debug > 0 {
				lib.Printf("GitHub Issue ID (before) '%d' --> '%v'\n", iid, cfg)
			}
			// Get separate org and repo
			ary := strings.Split(cfg.Repo, "/")
			if len(ary) != 2 {
				if ctx.Debug > 0 {
					lib.Printf("Warning: wrong repository name: %s\n", cfg.Repo)
				}
				return
			}
			// Use Github API to get issue info
			got := false
			for tr := 1; tr <= ctx.MaxGHAPIRetry; tr++ {
				_, rem, waitPeriod := lib.GetRateLimits(gctx, gc, true)
				if rem <= ctx.MinGHAPIPoints {
					if waitPeriod.Seconds() <= float64(ctx.MaxGHAPIWaitSeconds) {
						lib.Printf("API limit reached while getting issue data, waiting %v (%d)\n", waitPeriod, tr)
						time.Sleep(time.Duration(1) * time.Second)
						time.Sleep(waitPeriod)
						continue
					} else {
						lib.Fatalf("API limit reached while getting issue data, aborting, don't want to wait %v", waitPeriod)
						return
					}
				}
				issue, _, err := gc.Issues.Get(gctx, ary[0], ary[1], cfg.Number)
				lib.HandlePossibleError(err, &cfg, "Issues.Get")
				if issue.Milestone != nil {
					cfg.MilestoneID = issue.Milestone.ID
				}
				cfg.GhIssue = issue
				cfg.CreatedAt = time.Now()
				got = true
				break
			}
			if !got {
				lib.Fatalf("GetRateLimit call failed %d times while getting issue data, aboorting", ctx.MaxGHAPIRetry)
				return
			}

			// Use GitHub API to get labels info
			cfg.LabelsMap = make(map[int64]string)
			var (
				resp   *github.Response
				labels []*github.Label
			)
			for {
				got := false
				for tr := 1; tr <= ctx.MaxGHAPIRetry; tr++ {
					_, rem, waitPeriod := lib.GetRateLimits(gctx, gc, true)
					if rem <= ctx.MinGHAPIPoints {
						if waitPeriod.Seconds() <= float64(ctx.MaxGHAPIWaitSeconds) {
							lib.Printf("API limit reached while getting issue labels, waiting %v (%d)\n", waitPeriod, tr)
							time.Sleep(time.Duration(1) * time.Second)
							time.Sleep(waitPeriod)
							continue
						} else {
							lib.Fatalf("API limit reached while getting issue labels, aborting, don't want to wait %v", waitPeriod)
							return
						}
					}
					var errIn error
					labels, resp, errIn = gc.Issues.ListLabelsByIssue(gctx, ary[0], ary[1], cfg.Number, opt)
					lib.HandlePossibleError(errIn, &cfg, "Issues.ListLabelsByIssue")
					for _, label := range labels {
						cfg.LabelsMap[*label.ID] = *label.Name
					}
					got = true
					break
				}
				if !got {
					lib.Fatalf("GetRateLimit call failed %d times while getting issue labels, aboorting", ctx.MaxGHAPIRetry)
					return
				}

				// Handle eventual paging (should not happen for labels)
				if resp.NextPage == 0 {
					break
				}
				opt.Page = resp.NextPage
			}
			labelsAry := lib.Int64Ary{}
			for label := range cfg.LabelsMap {
				labelsAry = append(labelsAry, label)
			}
			sort.Sort(labelsAry)
			l := len(labelsAry)
			for i, label := range labelsAry {
				if i == l-1 {
					cfg.Labels += fmt.Sprintf("%d", label)
				} else {
					cfg.Labels += fmt.Sprintf("%d,", label)
				}
			}
			if ctx.Debug > 0 {
				lib.Printf("GitHub Issue ID (after) '%d' --> '%v'\n", iid, cfg)
			}

			// Finally update issues map, this must be protected by the mutex
			issuesMutex.Lock()
			issues[iid] = lib.IssueConfigAry{cfg}
			issuesMutex.Unlock()

			// Synchronize go routine
			if ch != nil {
				ch <- true
			}
		}(ch, key)
		// go routine called with 'ch' channel to sync and tag index

		nThreads++
		if nThreads == allowedThrN {
			<-ch
			nThreads--
			checked++
			// Get RateLimits info
			_, rem, wait := lib.GetRateLimits(gctx, gc, true)
			lib.ProgressInfo(checked, nIssues, dtStart, &lastTime, time.Duration(10)*time.Second, fmt.Sprintf("API points: %d, resets in: %v", rem, wait))
		}
	}
	// Usually all work happens on '<-ch'
	lib.Printf("Final GHAPI threads join\n")
	for nThreads > 0 {
		<-ch
		nThreads--
		checked++
		// Get RateLimits info
		_, rem, wait := lib.GetRateLimits(gctx, gc, true)
		lib.ProgressInfo(checked, nIssues, dtStart, &lastTime, time.Duration(10)*time.Second, fmt.Sprintf("API points: %d, resets in: %v", rem, wait))
	}

	lib.SyncIssuesState(gctx, gc, ctx, c, issues)
}

func main() {
	// Environment context parse
	var ctx lib.Ctx
	ctx.Init()

	dtStart := time.Now()
	// Clean unneeded events
	if !ctx.SkipArtificailClean {
		cleanArtificialEvents(&ctx)
	}

	// Create artificial events
	if !ctx.SkipGHAPI {
		ghapi2db(&ctx)
	}
	dtEnd := time.Now()
	lib.Printf("Time: %v\n", dtEnd.Sub(dtStart))
}
