-- Keep legacy recalls nullable: their actor cannot be reconstructed reliably.
-- No foreign key: deleting an account must not remove recall attribution.
ALTER TABLE messages ADD COLUMN recalled_by text;
