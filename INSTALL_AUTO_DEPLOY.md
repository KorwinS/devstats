# devstats installation on Ubuntu

Prerequisites:
- Ubuntu 18.04.
- [golang](https://golang.org).
    - `apt-get update`
    - `apt install golang` - this installs Go 1.10.
    - `apt install git psmisc jsonlint yamllint gcc`
    - `mkdir /data; mkdir /data/dev`
1. Configure Go:
    - For example add to `~/.bash_profile` and/or `~/.profile`:
     ```
     GOPATH=$HOME/data/dev; export GOPATH
     PATH=$PATH:$GOPATH/bin; export PATH
     ```
    - Logout and login again.
    - [golint](https://github.com/golang/lint): `go get -u github.com/golang/lint/golint`
    - [goimports](https://godoc.org/golang.org/x/tools/cmd/goimports): `go get golang.org/x/tools/cmd/goimports`
    - [goconst](https://github.com/jgautheron/goconst): `go get github.com/jgautheron/goconst/cmd/goconst`
    - [usedexports](https://github.com/jgautheron/usedexports): `go get github.com/jgautheron/usedexports`
    - [errcheck](https://github.com/kisielk/errcheck): `go get github.com/kisielk/errcheck`
    - Go InfluxDB client: install with: `go get github.com/influxdata/influxdb/client/v2`
    - Go Postgres client: install with: `go get github.com/lib/pq`
    - Go unicode text transform tools: install with: `go get golang.org/x/text/transform` and `go get golang.org/x/text/unicode/norm`
    - Go YAML parser library: install with: `go get gopkg.in/yaml.v2`
    - Go GitHub API client: `go get github.com/google/go-github/github`
    - Go OAuth2 client: `go get golang.org/x/oauth2`
    - Go SQLite3 client: `go get github.com/mattn/go-sqlite3`
2. Go to `$GOPATH/src/` and clone devstats there:
    - `git clone https://github.com/cncf/devstats.git`, cd `devstats`
    - Set reuse TCP connections (Golang InfluxDB may need this under heavy load): `./scripts/net_tcp_config.sh`
    - This variable can be unavailable on your system, ignore the warining if this is the case.
3. If you want to make changes and PRs, please clone `devstats` from GitHub UI, and clone your forked version instead, like this:
    - `git clone https://github.com/your_github_username/devstats.git`
6. Go to devstats directory, so you are in `~/dev/go/src/devstats` directory and compile binaries:
    - `make`
7. If compiled sucessfully then execute test coverage that doesn't need databases:
    - `make test`
    - Tests should pass.
8. Install binaries & metrics:
    - `sudo make install`
9. Install Postgres database ([link](https://gist.github.com/sgnl/609557ebacd3378f3b72)):
    - apt install postgresql (you can use specific version, for example `postgresql-9.6`)
    - sudo -i -u postgres
    - psql
    - Postgres only allows local connections by default so it is secure, we don't need to disable external connections:
    - Config file is: `/etc/postgresql/10/main/pg_hba.conf`, instructions to enable external connections (not recommended): `http://www.thegeekstuff.com/2014/02/enable-remote-postgresql-connection/?utm_source=tuicool`
    - Set bigger maximum number of connections, at least 200 or more: `/etc/postgresql/X.Y/main/postgresql.conf`. Default is 100. `max_connections = 300`.
    - `service postgresql restart`

66. Automatic deploy of first database (few examples)
    - Top deploy one of CNCF projects see `projects.yaml` chose one `projectname` and specify to deploy only this project: `ONLY=projectname`.
    - Use `INIT=1` to specify that you want to initialize logs database `devstats`, you need to provide passowrd for admin `PG_PASS`, `ro_user` (`PG_PASS_RO` used by Grafana for read-only access) and `devstats_team` (`PG_PASS_TEAM` - this is just another role to allow readonly access from the bastion ssh server).
    - You must use `INIT=1` when running for the first time, this creates shared logs database and postgres users.
    - This will generate all data, without fetching anything from backups: `INIT=1 ONLY=projectname PG_PASS=... PG_PASS_RO=... PG_PASS_TEAM=... IDB_PASS=... IDB_HOST=localhost ./devel/deploy_all.sh`
    - To deploy other project and using Influx backup from production specify `IGET=1` (get influx data form backup on `cncftest.io`). Specify GET=1 to get Postgres backup from `cncftest.io` too.
    - When fetching InfluxDb data from a remote server, you need to provide password for that server (cncftest.io) in this case by `IDB_PASS_SRC=...`
    - `ONLY=otherproject IGET=1 GET=1 PG_PASS=... IDB_PASS=... IDB_PASS_SRC=... IDB_HOST=localhost ./devel/deploy_all.sh`.
    - If you do not specify `ONLY="project1 project2 ... projectN"` it will deploy all projects defined in `projects.yaml`.
    - Other possible env variables used for automatic deploy are in the comments section in a first lines of deploy files: `devel/deploy_all.sh devel/deploy_proj.sh devel/create_databases.sh devel/init_database.sh devel/create_grafana.sh all/add_project.sh`.
    - You can also take a look at `ADDING_NEW_PROJECT.md` file for more info about setting up new projects.

12. Install InfluxDB time-series database ([link](https://docs.influxdata.com/influxdb/v0.9/introduction/installation/)):
    - Ubuntu 18 contains an old `influxdb` when installed by default `apt install influxdb`, so:
    - `wget https://dl.influxdata.com/influxdb/releases/influxdb_1.5.2_amd64.deb`
    - `sudo dpkg -i influxdb_1.5.2_amd64.deb`
    - `sudo service influxdb start`
    - Create InfluxDB users: `IDB_HOST="localhost" IDB_PASS='admin_password_here' IDB_PASS_RO='ro_user_password' ./grafana/influxdb_init.sh`.
    - InfluxDB has authentication disabled by default: edit config file `vim /etc/influxdb/influxdb.conf` and change section `[http]`: `auth-enabled = true`, `max-body-size = 0`, `[subscriber]`: `http-timeout = "300s"`, `write-concurrency = 96`, `[coordinator]`: `write-timeout = "60s"`.
    - If you want to disable external InfluxDB access (for any external IP, only localhost) follow those instructions [SECURE_INFLUXDB.md](https://github.com/cncf/devstats/blob/master/SECURE_INFLUXDB.md).
    - `sudo service influxdb restart`
    - Try it: `influx -host localhost -username gha_admin -password adminpwd`: `show databases`, `show users`.
13. Databases installed, you need to test if all works fine, use database test coverage:
/*
    - `GHA2DB_PROJECT=kubernetes IDB_DB=dbtest IDB_HOST="localhost" IDB_PASS=your_influx_pwd PG_DB=dbtest PG_PASS=your_postgres_pwd make dbtest`
    - Tests should pass.
*/

14. We have both databases running and Go tools installed, let's try to sync database dump from k8s.devstats.cncf.io manually:
    - We need to prefix call with `GHA2DB_LOCAL=1` to enable using tools from "./" directory
    - To import data for the first time (Influx database is empty and postgres database is at the state when Kubernetes SQL dump was made on [k8s.devstats.cncf.io](https://k8s.devstats.cncf.io)):
    - You need to have GitHub OAuth token, either put this token in `/etc/github/oauth` file or specify token value via `GHA2DB_GITHUB_OAUTH=deadbeef654...10a0` (here you token value)
    - If you really don't want to use GitHub OAuth2 token, specify `GHA2DB_GITHUB_OAUTH=-` - this will force tokenless operation (via public API), it is a lot more rate limited than OAuth2 which gives 5000 API points/h
    - `IDB_HOST="localhost" IDB_PASS=pwd PG_PASS=pwd ./kubernetes/reinit_all.sh`
    - This can take a while (depending how old is psql dump `gha.sql.xz` on [k8s.devstats.cncf.io](https://k8s.devstats.cncf.io). It is generated daily at 3:00 AM UTC.
    - Command should be successfull.

15. We need to setup cron job that will call sync every hour (10 minutes after 1:00, 2:00, ...)
    - You need to open `crontab.entry` file, it looks like this for single project setup (this is obsolete, please use `devstats` mode instead):
    ```
    8 * * * * PATH=$PATH:/path/to/your/GOPATH/bin GHA2DB_CMDDEBUG=1 GHA2DB_PROJECT=kubernetes IDB_HOST="localhost" IDB_PASS='...' PG_PASS='...' gha2db_sync 2>> /tmp/gha2db_sync.err 1>> /tmp/gha2db_sync.log
    20 3 * * * PATH=$PATH:/path/to/your/GOPATH/bin cron_db_backup.sh gha 2>> /tmp/gha2db_backup.err 1>> /tmp/gha2db_backup.log
    */5 * * * * PATH=$PATH:/path/to/your/GOPATH/bin GOPATH=/your/gopath GHA2DB_CMDDEBUG=1 GHA2DB_PROJECT_ROOT=/path/to/repo PG_PASS="..." GHA2DB_SKIP_FULL_DEPLOY=1 webhook 2>> /tmp/gha2db_webhook.err 1>> /tmp/gha2db_webhook.log
    ```
    - For multiple projects you can use `devstats` instead of `gha2db_sync` and `cron/cron_db_backup_all.sh` instead of `cron/cron_db_backup.sh`.
    ```
    7 * * * * PATH=$PATH:/path/to/GOPATH/bin IDB_HOST="localhost" IDB_PASS="..." PG_PASS="..." devstats 2>> /tmp/gha2db_sync.err 1>> /tmp/gha2db_sync.log
    30 3 * * * PATH=$PATH:/path/to/GOPATH/bin cron_db_backup_all.sh 2>> /tmp/gha2db_backup.err 1>> /tmp/gha2db_backup.log
    */5 * * * * PATH=$PATH:/path/to/GOPATH/bin GOPATH=/go/path GHA2DB_CMDDEBUG=1 GHA2DB_PROJECT_ROOT=/path/to/repo GHA2DB_DEPLOY_BRANCHES="production,master" PG_PASS=... GHA2DB_SKIP_FULL_DEPLOY=1 webhook 2>> /tmp/gha2db_webhook.err 1>> /tmp/gha2db_webhook.log
    ```
    - First crontab entry is for automatic GHA sync.
    - Second crontab entry is for automatic daily backup of GHA database.
    - Third crontab entry is for Continuous Deployment - this a Travis Web Hook listener server, it deploys project when specific conditions are met, details [here](https://github.com/cncf/devstats/blob/master/CONTINUOUS_DEPLOYMENT.md).
    - You need to change "..." PG_PASS, IDB_HOST and IDB_PASS to the real postgres password value and copy this line.
    - You need to change "/path/to/your/GOPATH/bin" to the value of "$GOPATH/bin", you cannot use $GOPATH in crontab directly.
    - Run `crontab -e` and put this line at the end of file and save.
    - Cron job will update Postgres and InfluxDB databases at 0:10, 1:10, ... 23:10 every day.
    - It outputs logs to `/tmp/gha2db_sync.out` and `/tmp/gha2db_sync.err` and also to gha Postgres database: into table `gha_logs`.
    - Check database values and logs about 15 minutes after full hours, like 14:15:
    - Check max event created date: `select max(created_at) from gha_events` and logs `select * from gha_logs order by dt desc limit 20`.

16. Install [Grafana](http://docs.grafana.org/installation/mac/) or use Docker to enable multiple Grafana instances, see [MULTIPROJECT.md](https://github.com/cncf/devstats/blob/master/MULTIPROJECT.md).

    - `sudo apt-get install -y adduser libfontconfig`
    - `sudo dpkg -i grafana_5.x.x_amd64.deb`
    - `sudo service grafana-server start`
    - Configure Grafana, as described [here](https://github.com/cncf/devstats/blob/master/GRAFANA.md).
    - `service grafana-server restart`
    - Go to Grafana UI (localhost:3000), choose sign out, and then access localhost:3000 again. You should be able to view dashboards as a guest. To login again use http://localhost:3000/login.
    - Install Apache as described [here](https://github.com/cncf/devstats/blob/master/APACHE.md).
    - You can also enable SSL, to do so you need to follow SSL instruction in [SSL](https://github.com/cncf/devstats/blob/master/SSL.md) (that requires domain name).

17. To change all Grafana page titles (starting with "Grafana - ") and icons use this script:
    - `GRAFANA_DATA=/usr/share/grafana/ ./grafana/{{project}}/change_title_and_icons.sh`.
    - `GRAFANA_DATA` can also be `/usr/share/grafana.prometheus/` for example, see [MULTIPROJECT.md](https://github.com/cncf/devstats/blob/master/MULTIPROJECT.md).
    - Replace `GRAFANA_DATA` with your Grafana data directory.
    - `service grafana-server restart`
    - In some cases browser and/or Grafana cache old settings in this case temporarily move Grafana's `settings.js` file:
    - `mv /usr/share/grafana/public/app/core/settings.js /usr/share/grafana/public/app/core/settings.js.old`, restart grafana server and restore file.
    - On Safari you can use Develop -> Empty Caches followed by refresh page (Command+R).

18. To enable Continuous deployment using Travis, please follow instructions [here](https://github.com/cncf/devstats/blob/master/CONTINUOUS_DEPLOYMENT.md).

19. You can create new metrics (as SQL files and YAML definitions) and dashboards in Grafana (export as JSON).
20. PRs and suggestions are welcome, please create PRs and Issues on the [GitHub](https://github.com/cncf/devstats).

# More details
- [Local Development](https://github.com/cncf/devstats/blob/master/DEVELOPMENT.md).
- [README](https://github.com/cncf/devstats/blob/master/README.md)
- [USAGE](https://github.com/cncf/devstats/blob/master/USAGE.md)
