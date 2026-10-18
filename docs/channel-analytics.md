# Channel analytics

Ready SQL for the channel audience log. The bot writes two tables that only grow:

- `channel_member_events`: one row per subscribe (`joined`), unsubscribe (`left`) and removal by an admin (`removed`), with the invite link and its name on joins.
- `channel_posts`: the time of every channel post.

Run the queries in any PostgreSQL client connected to the bot's `DATABASE_URL`. The cost of an ad is not in the database: divide it by the `stayed_30d` number of its link by hand.

## Before you read the numbers

- Logging starts when this version is deployed. Nobody who subscribed earlier has a `joined` row; a `left` without a `joined` before it means the user subscribed before logging started.
- Give every ad its own invite link with a unique name. Telegram hides the second half of links created by other admins, so the queries group by the name.
- Telegram keeps undelivered updates for 24 hours. If the bot is down longer, the changes in that gap are lost. An event whose database write fails is lost too (the bot logs the error). A lost leave makes a subscriber look subscribed.
- A subscription ends with `left` or `removed`. The post query counts only `left`: a removal is an admin's action, not a reaction to a post.
- Events carry Telegram's time to the second. When a join and a leave of one user share a second, the queries put the join first; the bot can save them in either order.
- Each photo of an album is saved as its own post with the same time. The post query folds posts of one second into the first of them.

## Ad links: joins and 30-day retention

Counts joins at least 30 days old, so every counted join had the full 30 days.

```sql
WITH events AS (
  SELECT e.*,
         LEAD(kind)        OVER w AS next_kind,
         LEAD(happened_at) OVER w AS next_at
  FROM channel_member_events e
  WINDOW w AS (PARTITION BY chat_id, user_id ORDER BY happened_at, kind = 'joined' DESC, id)
)
SELECT COALESCE(NULLIF(invite_link_name, ''), '(no link name)') AS link,
       COUNT(*) AS joins,
       COUNT(*) FILTER (
         WHERE next_kind IS NULL OR next_kind = 'joined'
            OR next_at > happened_at + interval '30 days'
       ) AS stayed_30d,
       ROUND(100.0 * COUNT(*) FILTER (
         WHERE next_kind IS NULL OR next_kind = 'joined'
            OR next_at > happened_at + interval '30 days'
       ) / COUNT(*), 1) AS stayed_pct
FROM events
WHERE kind = 'joined'
  AND happened_at <= now() - interval '30 days'
GROUP BY 1
ORDER BY joins DESC;
```

## Posts followed by unsubscribes

Each unsubscribe counts against the latest post before it, if that post is at most 24 hours older. One unsubscribe never counts against two posts.

```sql
WITH posts AS (
  -- An album arrives as one post per photo, all with the same time: keep the first.
  SELECT DISTINCT ON (chat_id, posted_at) chat_id, message_id, posted_at
  FROM channel_posts
  ORDER BY chat_id, posted_at, message_id
),
leaves AS (
  SELECT e.chat_id,
         (SELECT p.message_id
          FROM posts p
          WHERE p.chat_id = e.chat_id
            AND p.posted_at <= e.happened_at
            AND p.posted_at > e.happened_at - interval '24 hours'
          ORDER BY p.posted_at DESC
          LIMIT 1) AS message_id
  FROM channel_member_events e
  WHERE e.kind = 'left'
)
SELECT 'https://t.me/read_wine/' || p.message_id AS post,
       p.posted_at,
       COUNT(l.message_id) AS leaves_within_24h
FROM posts p
LEFT JOIN leaves l ON l.chat_id = p.chat_id AND l.message_id = p.message_id
GROUP BY p.chat_id, p.message_id, p.posted_at
ORDER BY leaves_within_24h DESC, p.posted_at DESC
LIMIT 20;
```

## How long subscribers stay

Every logged join, grouped by how long the subscription lasted.

```sql
WITH events AS (
  SELECT e.*,
         LEAD(kind)        OVER w AS next_kind,
         LEAD(happened_at) OVER w AS next_at
  FROM channel_member_events e
  WINDOW w AS (PARTITION BY chat_id, user_id ORDER BY happened_at, kind = 'joined' DESC, id)
),
lifetimes AS (
  SELECT CASE
           WHEN next_kind IS NULL OR next_kind = 'joined' THEN NULL
           ELSE next_at - happened_at
         END AS lasted
  FROM events
  WHERE kind = 'joined'
)
SELECT CASE
         WHEN lasted IS NULL                 THEN '5. still subscribed'
         WHEN lasted < interval '1 day'      THEN '1. under 1 day'
         WHEN lasted < interval '7 days'     THEN '2. 1 to 7 days'
         WHEN lasted < interval '30 days'    THEN '3. 7 to 30 days'
         ELSE                                     '4. 30 days or more'
       END AS lifetime,
       COUNT(*) AS subscriptions
FROM lifetimes
GROUP BY 1
ORDER BY 1;
```

Median lifetime of the subscriptions that already ended:

```sql
WITH events AS (
  SELECT e.*,
         LEAD(kind)        OVER w AS next_kind,
         LEAD(happened_at) OVER w AS next_at
  FROM channel_member_events e
  WINDOW w AS (PARTITION BY chat_id, user_id ORDER BY happened_at, kind = 'joined' DESC, id)
)
SELECT COUNT(*) AS ended,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY next_at - happened_at) AS median_lifetime
FROM events
WHERE kind = 'joined' AND next_kind IN ('left', 'removed');
```
