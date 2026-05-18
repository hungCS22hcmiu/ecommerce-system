-- Drop the partial index that references the enum type (required before type conversion)
DROP INDEX IF EXISTS idx_notifications_status;

-- Drop defaults tied to the PostgreSQL enum types before altering columns
ALTER TABLE notifications ALTER COLUMN type   DROP DEFAULT;
ALTER TABLE notifications ALTER COLUMN status DROP DEFAULT;

-- Convert enum columns to varchar so IN_APP can be stored without ALTER TYPE issues
ALTER TABLE notifications ALTER COLUMN type   TYPE VARCHAR(50) USING type::text;
ALTER TABLE notifications ALTER COLUMN status TYPE VARCHAR(50) USING status::text;

DROP TYPE notification_type;
DROP TYPE notification_status;

-- Add in-app notification columns
ALTER TABLE notifications
  ADD COLUMN title   VARCHAR(255),
  ADD COLUMN is_read BOOLEAN NOT NULL DEFAULT FALSE;

-- Partial index for fast unread count queries per user
CREATE INDEX idx_notifications_user_unread ON notifications(user_id) WHERE is_read = false;
