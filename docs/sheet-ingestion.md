# Sheet ingestion — plan export

In production this becomes multi-tenant Google Sheets ingestion. The demo simulates Sheets behind a stable interface so the export UX is real without OAuth plumbing.

## Interface

```go
type SheetClient interface {
    WritePlan(ctx context.Context, userID string, planID string) (SheetRef, error)
}
```

## Demo backend

`MockSheets` writes a CSV to S3 and returns a URL. The URL is rendered in the UI as a "Spreadsheet" panel with the same column layout Google Sheets would show.

## Production migration

1. Add OAuth flow per workspace.
2. Replace `MockSheets` with `GoogleSheets` implementing the same interface.
3. Add a per-user `sheets_token` encrypted column.
4. Add a `SHEETS_DAILY_QUOTA` check.

The agent code does not need to change.

## Plan schema

```sql
CREATE TABLE plans (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       STRING NOT NULL,
  title         STRING NOT NULL,
  created_at    TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE plan_items (
  plan_id       UUID REFERENCES plans(id),
  day_offset    INT,
  slot          STRING,        -- 'morning' | 'midday' | 'evening'
  method        STRING,        -- 'box_breath','body_scan','walk', ...
  duration_min  INT,
  notes         STRING,
  PRIMARY KEY (plan_id, day_offset, slot)
);
```

## CSV format

```csv
day,slot,method,duration_min,notes
1,morning,box_breath,5,steady pace
1,evening,body_scan,10,lower lights
...
```

## Mock Sheets UI

The frontend renders the CSV in a table styled like a Google Sheet. The "Open" button copies a public S3 URL that the user can interact with.
