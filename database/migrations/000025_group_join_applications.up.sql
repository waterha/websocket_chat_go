-- Distinguish self-service membership applications from administrator invites.
-- Existing rows are invites by default; identify legacy application rows using
-- the marker that was historically prefixed to their message.
ALTER TABLE conversation_join_requests
    ADD COLUMN is_application boolean NOT NULL DEFAULT false;

UPDATE conversation_join_requests
SET is_application = true,
    message = substr(message, 7)
WHERE message LIKE '加入申请:%';
