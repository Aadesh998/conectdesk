-- Extensions and enum types.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

DROP TYPE IF EXISTS "channels" CASCADE; CREATE TYPE "channels" AS ENUM ('email', 'livechat', 'whatsapp', 'messenger', 'instagram', 'telegram', 'voice');
DROP TYPE IF EXISTS "message_type" CASCADE; CREATE TYPE "message_type" AS ENUM ('incoming','outgoing','activity');
DROP TYPE IF EXISTS "message_sender_type" CASCADE; CREATE TYPE "message_sender_type" AS ENUM ('agent','contact');
DROP TYPE IF EXISTS "message_status" CASCADE; CREATE TYPE "message_status" AS ENUM ('received','sent','failed','pending');
DROP TYPE IF EXISTS "content_type" CASCADE; CREATE TYPE "content_type" AS ENUM ('text','html');
DROP TYPE IF EXISTS "conversation_assignment_type" CASCADE; CREATE TYPE "conversation_assignment_type" AS ENUM ('Round robin','Manual');
DROP TYPE IF EXISTS "template_type" CASCADE; CREATE TYPE "template_type" AS ENUM ('email_outgoing', 'email_notification');
-- Visitors are unauthenticated contacts.
DROP TYPE IF EXISTS "user_type" CASCADE; CREATE TYPE "user_type" AS ENUM ('agent', 'contact', 'visitor', 'ai_assistant');
DROP TYPE IF EXISTS "ai_provider" CASCADE; CREATE TYPE "ai_provider" AS ENUM ('openai');
DROP TYPE IF EXISTS "automation_execution_mode" CASCADE; CREATE TYPE "automation_execution_mode" AS ENUM ('all', 'first_match');
DROP TYPE IF EXISTS "macro_visibility" CASCADE; CREATE TYPE "macro_visibility" AS ENUM ('all', 'team', 'user');
DROP TYPE IF EXISTS "view_visibility" CASCADE; CREATE TYPE "view_visibility" AS ENUM ('all', 'team', 'user');
DROP TYPE IF EXISTS "media_disposition" CASCADE; CREATE TYPE "media_disposition" AS ENUM ('inline', 'attachment');
DROP TYPE IF EXISTS "media_store" CASCADE; CREATE TYPE "media_store" AS ENUM ('s3', 'fs');
DROP TYPE IF EXISTS "user_availability_status" CASCADE; CREATE TYPE "user_availability_status" AS ENUM ('online', 'away', 'away_manual', 'offline', 'away_and_reassigning');
DROP TYPE IF EXISTS "applied_sla_status" CASCADE; CREATE TYPE "applied_sla_status" AS ENUM ('pending', 'breached', 'met', 'partially_met');
DROP TYPE IF EXISTS "sla_event_status" CASCADE; CREATE TYPE "sla_event_status" AS ENUM ('pending', 'breached', 'met');
DROP TYPE IF EXISTS "sla_metric" CASCADE; CREATE TYPE "sla_metric" AS ENUM ('first_response', 'resolution', 'next_response');
DROP TYPE IF EXISTS "sla_notification_type" CASCADE; CREATE TYPE "sla_notification_type" AS ENUM ('warning', 'breach');
DROP TYPE IF EXISTS "activity_log_type" CASCADE; CREATE TYPE "activity_log_type" AS ENUM ('agent_login', 'agent_logout', 'agent_away', 'agent_away_reassigned', 'agent_online', 'agent_password_set', 'agent_role_permissions_changed', 'contact_deleted', 'contact_data_exported', 'platform_impersonation', 'agent_2fa_enabled', 'agent_2fa_disabled', 'data_exported', 'retention_policy_changed', 'retention_applied');
DROP TYPE IF EXISTS "macro_visible_when" CASCADE; CREATE TYPE "macro_visible_when" AS ENUM ('replying', 'starting_conversation', 'adding_private_note');
DROP TYPE IF EXISTS "user_notification_type" CASCADE; CREATE TYPE "user_notification_type" AS ENUM ('mention', 'assignment', 'sla_warning', 'sla_breach', 'call_back', 'call_back_overdue');
DROP TYPE IF EXISTS "conversation_status_category" CASCADE; CREATE TYPE "conversation_status_category" AS ENUM ('open', 'waiting', 'resolved');
DROP TYPE IF EXISTS "ai_knowledge_type" CASCADE; CREATE TYPE "ai_knowledge_type" AS ENUM ('snippet');
DROP TYPE IF EXISTS "webhook_event" CASCADE; CREATE TYPE webhook_event AS ENUM (
	'conversation.created',
	'conversation.status_changed',
	'conversation.tags_changed',
	'conversation.assigned',
	'conversation.unassigned',
	'message.created',
	'message.updated'
);
