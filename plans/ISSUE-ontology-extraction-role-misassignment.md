# ISSUE: Ontology Extraction Assigns Wrong Roles to FK and Dimension Columns

Status: FIXED

## Observed

After running ontology extraction on the `ekaya_marketing` database, `get_ontology` at `tables` depth shows several columns with `role: "primary_key"` that are not primary keys:

- `content_posts.app_id` — FK to `applications.id`, should be `dimension` or `identifier`
- `content_posts.week_number` — integer dimension, not a PK
- `paid_placements.channel_id` — FK to `paid_channels.id`
- `paid_placements.task_id` — FK to `marketing_tasks.id`
- `post_channel_steps.post_id` — FK to `content_posts.id`
- `post_channel_steps.day_offset` — integer dimension, not a PK
- `marketing_tasks.task_number` — unique integer but not the PK (id is)
- `marketing_tasks.target_week` — integer dimension
- `marketing_tasks.app_id` — FK to `applications.id`
- `marketing_task_dependencies.blocking_task_id` — FK to `marketing_tasks.id`
- `marketing_task_dependencies.blocked_task_id` — FK to `marketing_tasks.id`
- `directory_submissions.directory_id` — FK to `mcp_directories.id`
- `lead_magnet_leads.post_id` — FK to `content_posts.id`

## Steps to Reproduce

1. Create tables with `id SERIAL PRIMARY KEY` and integer FK columns
2. Run ontology extraction
3. Call `get_ontology` at `tables` depth
4. Observe FK and integer columns tagged as `role: "primary_key"`

## Expected

- Columns with actual PK constraints → `role: "primary_key"`
- FK columns → `role: "identifier"` or `role: "dimension"`
- Integer dimension columns (week_number, day_offset, target_week) → `role: "dimension"`

## Likely Cause

The extraction logic may be using heuristics (e.g. integer + NOT NULL + column name ends in `_id`) rather than checking the actual `is_primary_key` schema metadata, which is correctly set to `false` on these columns.
