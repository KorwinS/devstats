# Kubernetes reviewers dashboard (part of GitHub stats dashboard)

Links:
- Postgres SQL file: [github_stats_by_repos.sql](https://github.com/cncf/devstats/blob/master/metrics/kubernetes/github_stats_by_repos.sql).
- Postgres SQL file: [github_stats_by_repo_groups.sql](https://github.com/cncf/devstats/blob/master/metrics/kubernetes/github_stats_by_repo_groups.sql).
- InfluxDB series definition: [metrics.yaml](https://github.com/cncf/devstats/blob/master/metrics/kubernetes/metrics.yaml) (search for `github_stats_by_repo`).
- Grafana dashboard JSON: [github-stats-by-repository.json](https://github.com/cncf/devstats/blob/master/grafana/dashboards/kubernetes/github-stats-by-repository.json).
- Grafana dashboard JSON: [github-stats-by-repository-group.json](https://github.com/cncf/devstats/blob/master/grafana/dashboards/kubernetes/github-stats-by-repository-group.json).
- User documentation: [reviewers.md](https://github.com/cncf/devstats/blob/master/docs/dashboards/kubernetes/reviewers.md).
- Production version: [view stats by repos](https://k8s.devstats.cncf.io/d/49/github-stats-by-repository?orgId=1), [view stats by repo groups](https://k8s.devstats.cncf.io/d/48/github-stats-by-repository-group?orgId=1).
- Test version: [view stats by repos](https://k8s.cncftest.io/d/49/github-stats-by-repository?orgId=1), [view stats by repo groups](https://k8s.cncftest.io/d/48/github-stats-by-repository-group?orgId=1).

# Description

- We're quering `gha_texts` table. It contains all 'texts' from all Kubernetes repositories.
- For more information about `gha_texts` table please check: [docs/tables/gha_texts.md](https://github.com/cncf/devstats/blob/master/docs/tables/gha_texts.md).
- We're creating temporary table 'matching' which contains all event IDs that contain `/lgtm` or `/approve` (no case sensitive) in a separate line (there can be more lines before and/or after this line).
- The exact psql regexp is: `(?i)(?:^|\n|\r)\s*/(?:lgtm|approve)\s*(?:\n|\r|$)`.
- We're only looking for texts created between `{{from}}` and `{{to}}` dates. Values for `from` and `to` will be replaced with final periods described later.
- We are counting actors who added text matching given regexp.
- Then we're creating 'reviews' object (used by `with` clause) that contains all event IDs (`gha_events`) that belong to GitHub event type: `PullRequestReviewCommentEvent`.
- For more information about `gha_events` table please check: [docs/tables/gha_event.md](https://github.com/cncf/devstats/blob/master/docs/tables/gha_events.md).
- Then comes the final select which returns multiple rows for either repositories or repository groups.
- Each row returns single value, so the metric type is: `multi_row_single_column`.
- Each row is in the format column 1: `gh_stats_repo_groups_reviewers,RepoGroupName`, column 2: `NumberOfReviewersInThisRepoGroup`. This is for repo groups version.
- Each row is in the format column 1: `gh_stats_repos_reviewers,RepoName`, column 2: `NumberOfReviewersInThisRepo`. This is for repos version.
- There are other rows with data for other stats. For example `gh_stats_repo_groups_commits,RepoGroupName` etc.
- Value for each repository group or repository is calculated as a number of distinct actor logins.
- We are checking that author is not a bot (see [excluding bots](https://github.com/cncf/devstats/blob/master/docs/excluding_bots.md))
- We are counting actors who added `lgtm` or `approve` label in a given period (`gha_issues_events_labels` table).
- For more information about `gha_issues_events_labels` table please check: [docs/tables/gha_issues_events_labels.md](https://github.com/cncf/devstats/blob/master/docs/tables/gha_issues_events_labels.md).
- We are counting actors who added PR review comment (event type `PullRequestReviewCommentEvent`).
- Event belong to a given repository group or repository.
- For repository group definition check: [repository groups](https://github.com/cncf/devstats/blob/master/docs/repository_groups.md) (table `gha_events` and commit files for file level granularity repo groups).
- For more information about `gha_repos` table please check: [docs/tables/gha_repos.md](https://github.com/cncf/devstats/blob/master/docs/tables/gha_repos.md).

# Periods and Influx series

Metric usage is defined in metric.yaml as follows:
```
- name: Github Stats by Repository Group
  series_name_or_func: multi_row_single_column
  sql: github_stats_by_repo_groups
  periods: h,d,w,m,q,y
  aggregate: 1,3,4,7,24
  skip: h7,w7,m7,q7,y7,h3,d3,w3,q3,y3,h4,d4,y4,d24,w24,m24,q24,y24
  multi_value: true
- name: Github Stats by Repository
  series_name_or_func: multi_row_single_column
  sql: github_stats_by_repos
  periods: h,d,w,m,q,y
  aggregate: 1,3,4,7,24
  skip: h7,w7,m7,q7,y7,h3,d3,w3,q3,y3,h4,d4,y4,d24,w24,m24,q24,y24
  multi_value: true
```
- It means that we should call Postgres metric [github_stats_by_repo_groups.sql](https://github.com/cncf/devstats/blob/master/metrics/kubernetes/github_stats_by_repo_groups.sql) and [github_stats_by_repos.sql](https://github.com/cncf/devstats/blob/master/metrics/kubernetes/github_stats_by_repos.sql).
- We should expect multiple rows each with 2 columns: 1st defines output Influx series name, 2nd defines value.
- We're using `multivalue: true` which means first column will contain multivalued series definition. It is comma `,` separated. Influx series name comes first, and then series value name.
- For example 1st column: `'gh_stats_repo_groups_reviewers,Kubernetes'`, 2nd column: `20.0` will create series named `gh_stats_repo_groups_reviewers` with column `Kubernetes` with value `20.0`.
- This SQLs calculate many metrics in addition to reviewers, general series name will be `gh_stats_repo_groups_{{stat}}` and `gh_stats_repos_{{stat}}`, we're only describing `{{stat}} = reviewers` case in this documentation.
- See [here](https://github.com/cncf/devstats/blob/master/docs/periods.md) for periods definitions.
- The final InfluxDB series name would be (for repo groups version): `gh_stats_repo_groups_[[stat]]_[[period]]` and column anmes from `[[repogroup]]`.
- Value of `[[period]]` will be from d,w,m,q,y,d7 and `[[repogroup]]` will be from 'all,apps,contrib,kubernetes,...', see [repository groups](https://github.com/cncf/devstats/blob/master/docs/repository_groups.md) for details.
- The final InfluxDB series name would be (for repos version): `gh_stats_repos_[[stat]]_[[period]]` and column names from`[[repo]]` that will be from one of the Kubernetes projects repo name (with special characters changed to `_`, for example `kubernetes_kubernetes`).
- Repo group name and repo name returned by Postgres SQL is normalized (downcased, removed special chars etc.) to be usable as a Influx series name [here](https://github.com/cncf/devstats/blob/master/cmd/db2influx/db2influx.go#L112) using [this](https://github.com/cncf/devstats/blob/master/unicode.go#L23).
- Final query is [here](https://github.com/cncf/devstats/blob/master/grafana/dashboards/kubernetes/github-stats-by-repository.json) or [there](https://github.com/cncf/devstats/blob/master/grafana/dashboards/kubernetes/github-stats-by-repository-group.json), search for `gh_stats_repo`.
- `$timeFiler` value comes from Grafana date range selector. It is handled by Grafana internally.
- `[[period]]` comes from variable definition in dashboard JSON, search for `"period"`.
- `[[repogroup]]` or `[[repo]]` comes from Grafana variable that uses influx tags values, search for `repos` or `repogroups`.
- For repos we're using repo [aliases](https://github.com/cncf/devstats/blob/master/docs/repository_aliases.md) to avoid duplicating renamed repositories.
- To see more details about repository group tags, and all other tags check [tags.md](https://github.com/cncf/devstats/blob/master/docs/tags.md).
- Releases comes from Grafana annotations: search for `annotations` in the dashboard JSON.
- For more details about annotations check [here](https://github.com/cncf/devstats/blob/master/docs/annotations.md).
- Project name is customized per project, it uses `[[full_name]]` template variable.
- Per project variables are defined using `idb_vars`, `pdb_vars` tools, more info [here](https://github.com/cncf/devstats/blob/master/docs/vars.md).
- Grafana documentation for this dashboard depends on `Statistic` selection, variable `[[stat]]` is used to display documentation for a given statistic.
