select
  sub.id,
  coalesce(string_agg(sub.email, ', '), sub.id || '@users.noreply.github.com') as email
from (
  select distinct a.login as id,
    ae.email
  from
    gha_events e,
    gha_actors a
  left join
    gha_actors_emails ae
  on
    ae.actor_id = a.id
  where
    (e.actor_id = a.id or e.dup_actor_login = a.login)
    and type in (
      'PushEvent', 'PullRequestEvent', 'IssuesEvent'
    )
  order by
    id asc,
    email asc
  ) sub
group by
  sub.id
;
