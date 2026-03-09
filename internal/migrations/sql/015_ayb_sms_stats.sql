-- Migration 015: SMS confirmation stats columns
-- Adds confirm_count and fail_count to daily counts for conversion tracking.

ALTER TABLE _ayb_sms_daily_counts ADD COLUMN IF NOT EXISTS confirm_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE _ayb_sms_daily_counts ADD COLUMN IF NOT EXISTS fail_count INTEGER NOT NULL DEFAULT 0;
