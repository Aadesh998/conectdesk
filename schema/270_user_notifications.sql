-- In-app user notifications.

DROP TABLE IF EXISTS user_notifications CASCADE;
CREATE TABLE user_notifications (
	tenant_id INT NOT NULL DEFAULT 1 REFERENCES tenants(id) ON DELETE CASCADE ON UPDATE CASCADE,
	id SERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ DEFAULT NOW(),
	updated_at TIMESTAMPTZ DEFAULT NOW(),
	user_id BIGINT REFERENCES users(id) ON DELETE CASCADE ON UPDATE CASCADE NOT NULL,
	notification_type user_notification_type NOT NULL,
	title TEXT NOT NULL,
	body TEXT NULL,
	is_read BOOLEAN DEFAULT FALSE NOT NULL,
	conversation_id BIGINT REFERENCES conversations(id) ON DELETE CASCADE ON UPDATE CASCADE,
	message_id BIGINT REFERENCES conversation_messages(id) ON DELETE CASCADE ON UPDATE CASCADE,
	actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL ON UPDATE CASCADE,
	meta JSONB DEFAULT '{}'::jsonb NOT NULL,
	CONSTRAINT constraint_user_notifications_on_title CHECK (length(title) <= 500),
	CONSTRAINT constraint_user_notifications_on_body CHECK (length(body) <= 2000)
);
CREATE INDEX index_user_notifications_on_user_id ON user_notifications(user_id);
CREATE INDEX index_user_notifications_on_user_id_is_read ON user_notifications(user_id, is_read);
CREATE INDEX index_user_notifications_on_created_at ON user_notifications(created_at);
CREATE INDEX index_user_notifications_on_conversation_id ON user_notifications(conversation_id);
CREATE INDEX index_user_notifications_on_tenant_id ON user_notifications (tenant_id);
