-- seed_tenant creates a tenant's default data: roles, statuses, priorities,
-- settings, templates, business hours, SLA policy and AI prompts and providers.
-- Installing seeds the default tenant; signing up seeds each new tenant.
-- The body isn't indented, so the template bodies it stores stay unchanged.
CREATE OR REPLACE FUNCTION seed_tenant(p_tenant_id INT) RETURNS void AS $seed$
BEGIN
INSERT INTO ai_providers
(tenant_id, "name", provider, type, config, is_default)
VALUES
(p_tenant_id, 'openai', 'openai', 'completion', '{"api_key": ""}'::jsonb, true),
(p_tenant_id, 'embedding', 'openai', 'embedding', '{"api_key": ""}'::jsonb, false);

-- Default AI prompts
INSERT INTO ai_prompts (tenant_id, "key", "content", title)
VALUES
(p_tenant_id, 'make_friendly', 'Modify the text to make it more friendly and approachable.', 'Make Friendly'),
(p_tenant_id, 'make_concise', 'Simplify the text to make it more concise and to the point.', 'Make Concise'),
(p_tenant_id, 'add_empathy', 'Add empathy to the text while retaining the original meaning.', 'Add Empathy'),
(p_tenant_id, 'adjust_positive_tone', 'Adjust the tone of the text to make it sound more positive and reassuring.', 'Adjust Positive Tone'),
(p_tenant_id, 'make_professional', 'Rephrase the text to make it sound more formal and professional and to the point.', 'Make Professional'),
(p_tenant_id, 'fix_grammar_spelling', 'Fix any spelling and grammar mistakes in the text while retaining the original meaning and tone.', 'Fix Grammar & Spelling');

-- Default settings
INSERT INTO settings (tenant_id, "key", value)
VALUES
    (p_tenant_id, 'app.lang', '"en-US"'::jsonb),
    (p_tenant_id, 'app.root_url', '"http://localhost:9000"'::jsonb),
    (p_tenant_id, 'app.logo_url', '""'::jsonb),
    (p_tenant_id, 'app.site_name', '"ConectDesk"'::jsonb),
    (p_tenant_id, 'app.favicon_url', '"http://localhost:9000/favicon.ico"'::jsonb),
    (p_tenant_id, 'app.max_file_upload_size', '20'::jsonb),
    (p_tenant_id, 'app.require_2fa', 'false'::jsonb),
    (p_tenant_id, 'app.require_sso', 'false'::jsonb),
    (p_tenant_id, 'app.allowed_file_upload_extensions', '["*"]'::jsonb),
	(p_tenant_id, 'app.timezone', '"Asia/Kolkata"'::jsonb),
	(p_tenant_id, 'app.business_hours_id', '""'::jsonb),
	(p_tenant_id, 'app.show_conversation_subject', 'true'::jsonb),
	(p_tenant_id, 'ai_agent.faq_learning_enabled', 'false'::jsonb),
	(p_tenant_id, 'privacy.retention_days', '0'::jsonb),
	(p_tenant_id, 'privacy.retention_delete_contacts', 'false'::jsonb),
	(p_tenant_id, 'onboarding.dismissed', 'false'::jsonb),
	(p_tenant_id, 'leads.stages', '[{"key": "new", "label": "New", "kind": "open"}, {"key": "interested", "label": "Interested", "kind": "open"}, {"key": "visit_booked", "label": "Visit booked", "kind": "open"}, {"key": "admitted", "label": "Admitted", "kind": "won"}, {"key": "lost", "label": "Lost", "kind": "lost"}]'::jsonb),
	(p_tenant_id, 'leads.auto_new', 'true'::jsonb),
	(p_tenant_id, 'daily_report.enabled', 'false'::jsonb),
	(p_tenant_id, 'daily_report.time', '"20:00"'::jsonb),
	(p_tenant_id, 'daily_report.whatsapp_inbox_id', '0'::jsonb),
	(p_tenant_id, 'daily_report.template_name', '""'::jsonb),
	(p_tenant_id, 'daily_report.template_language', '"en"'::jsonb),
	(p_tenant_id, 'daily_report.recipients', '[]'::jsonb),
	(p_tenant_id, 'ai_features.call_summaries', 'false'::jsonb),
	(p_tenant_id, 'ai_features.lead_extraction', 'false'::jsonb),
	(p_tenant_id, 'jev.enabled', 'false'::jsonb),
	(p_tenant_id, 'jev.api_key', '""'::jsonb),
	(p_tenant_id, 'jev.model', '"jev-latest"'::jsonb),
    (p_tenant_id, 'notification.email.username', '"admin@yourcompany.com"'::jsonb),
    (p_tenant_id, 'notification.email.host', '""'::jsonb),
    (p_tenant_id, 'notification.email.port', '587'::jsonb),
    (p_tenant_id, 'notification.email.password', '""'::jsonb),
    (p_tenant_id, 'notification.email.max_conns', '5'::jsonb),
    (p_tenant_id, 'notification.email.idle_timeout', '"25s"'::jsonb),
    (p_tenant_id, 'notification.email.wait_timeout', '"60s"'::jsonb),
    (p_tenant_id, 'notification.email.auth_protocol', '"plain"'::jsonb),
	(p_tenant_id, 'notification.email.tls_type', '"starttls"'::jsonb),
	(p_tenant_id, 'notification.email.tls_skip_verify', 'false'::jsonb),
	(p_tenant_id, 'notification.email.hello_hostname', '""'::jsonb),
    (p_tenant_id, 'notification.email.email_address', '"admin@yourcompany.com"'::jsonb),
    (p_tenant_id, 'notification.email.max_msg_retries', '3'::jsonb),
    (p_tenant_id, 'notification.email.enabled', 'false'::jsonb);

-- Default conversation priorities
INSERT INTO conversation_priorities (tenant_id, name) VALUES
(p_tenant_id, 'Low'),
(p_tenant_id, 'Medium'),
(p_tenant_id, 'High');

-- Default conversation statuses
INSERT INTO conversation_statuses (tenant_id, name, category) VALUES
(p_tenant_id, 'Open', 'open'),
(p_tenant_id, 'Snoozed', 'waiting'),
(p_tenant_id, 'Resolved', 'resolved'),
(p_tenant_id, 'Closed', 'resolved');

-- Default roles
INSERT INTO
	roles (tenant_id, "name", description, permissions)
VALUES
	(p_tenant_id, 
		'Agent',
		'Role for all agents with limited access to conversations.',
		'{conversations:read_all,conversations:read_unassigned,conversations:read_assigned,conversations:read_team_inbox,conversations:read_team_all,conversations:read,conversations:update_user_assignee,conversations:update_team_assignee,conversations:update_priority,conversations:update_status,conversations:update_tags,messages:read,messages:write,view:manage}'
	);

INSERT INTO
	roles (tenant_id, "name", description, permissions)
VALUES
	(p_tenant_id, 
		'Admin',
		'Role for users who have complete access to everything.',
		'{webhooks:manage,context_links:manage,activity_logs:manage,custom_attributes:manage,contacts:read_all,contacts:read,contacts:write,contacts:block,contacts:delete,contacts:export,contact_notes:read,contact_notes:write,contact_notes:delete,conversations:write,ai:manage,help_center:manage,general_settings:manage,notification_settings:manage,oidc:manage,conversations:read_all,conversations:read_unassigned,conversations:read_assigned,conversations:read_team_inbox,conversations:read_team_all,conversations:read,conversations:update_user_assignee,conversations:update_team_assignee,conversations:update_priority,conversations:update_status,conversations:update_tags,messages:read,messages:write,view:manage,shared_views:manage,status:manage,tags:manage,macros:manage,users:manage,teams:manage,automations:manage,inboxes:manage,roles:manage,reports:manage,templates:manage,business_hours:manage,sla:manage}'
	);


-- Email notification templates
INSERT INTO templates
(tenant_id, "type", body, is_default, "name", subject, is_builtin)
VALUES(p_tenant_id, 'email_notification'::template_type, '
<p>A new conversation has been assigned to you:</p>

<div>
    Reference number: {{ .Conversation.ReferenceNumber }} <br>
    Subject: {{ .Conversation.Subject }}
</div>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>

<div>
    Best regards,<br>
    ConectDesk
</div>

', false, 'Conversation assigned', 'New conversation assigned to you', true);

INSERT INTO templates
(tenant_id, "type", body, is_default, "name", subject, is_builtin)
VALUES (p_tenant_id, 
  'email_notification'::template_type,
  '

<p>This is a notification that the SLA for conversation {{ .Conversation.ReferenceNumber }} is approaching the SLA deadline for {{ .SLA.Metric }}.</p>

<p>
  Details:<br>
  - Conversation reference number: {{ .Conversation.ReferenceNumber }}<br>
  - Metric: {{ .SLA.Metric }}<br>
  - Due in: {{ .SLA.DueIn }}
</p>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>


<p>
  Best regards,<br>
  ConectDesk
</p>

',
  false,
  'SLA breach warning',
  'SLA Alert: Conversation {{ .Conversation.ReferenceNumber }} is approaching SLA deadline for {{ .SLA.Metric }}',
  true
);

INSERT INTO templates
(tenant_id, "type", body, is_default, "name", subject, is_builtin)
VALUES (p_tenant_id, 
  'email_notification'::template_type,
  '
<p>This is an urgent alert that the SLA for conversation {{ .Conversation.ReferenceNumber }} has been breached for {{ .SLA.Metric }}. Please take immediate action.</p>

<p>
  Details:<br>
  - Conversation reference number: {{ .Conversation.ReferenceNumber }}<br>
  - Metric: {{ .SLA.Metric }}<br>
  - Overdue by: {{ .SLA.OverdueBy }}
</p>

<p>
    <a href="{{ RootURL }}/inboxes/assigned/conversation/{{ .Conversation.UUID }}">View Conversation</a>
</p>


<p>
  Best regards,<br>
  ConectDesk
</p>

',
  false,
  'SLA breached',
  'Urgent: SLA Breach for Conversation {{ .Conversation.ReferenceNumber }} for {{ .SLA.Metric }}',
  true
);

INSERT INTO templates
(tenant_id, "type", body, is_default, "name", subject, is_builtin)
VALUES (p_tenant_id, 
  'email_notification'::template_type,
  '
<p>{{ .MentionedBy.FullName }} mentioned you in a private note on conversation #{{ .Conversation.ReferenceNumber }}.</p>

<blockquote style="background-color: #f5f5f5; padding: 12px; margin: 16px 0; border-left: 4px solid #ddd;">
{{ .Message.Content }}
</blockquote>

<p>
<a href="{{ RootURL }}/inboxes/mentioned/conversation/{{ .Conversation.UUID }}?scrollTo={{ .Message.UUID }}">View Conversation</a>
</p>

<p>
Best regards,<br>
conectdesk
</p>
',
  false,
  'Mentioned in conversation',
  '{{ .MentionedBy.FullName }} mentioned you in conversation #{{ .Conversation.ReferenceNumber }}',
  true
);

INSERT INTO templates
(tenant_id, "type", body, is_default, "name", subject, is_builtin)
VALUES (p_tenant_id, 
  'email_notification'::template_type,
  '
<p style="margin: 0 0 4px; font-size: 15px; color: #374151; text-align: center; line-height: 1.5;">
  Your conversation <strong style="color: #111827;">#{{ .Conversation.ReferenceNumber }}</strong> has been resolved.
</p>
<p style="margin: 0 0 28px; font-size: 13px; color: #9ca3af; text-align: center;">
  We would love to hear how it went.
</p>
<p style="margin: 0 0 20px; font-size: 14px; font-weight: 600; color: #374151; text-align: center;">
  How would you rate your experience?
</p>
<!-- Variable CSATUUID is also available -->
<div style="text-align: center; margin: 0 auto; max-width: 400px; font-size: 0;">
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=1" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128546;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Poor</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=2" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128533;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Fair</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=3" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128522;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Good</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=4" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#128515;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Great</span>
    </a>
  </div>
  <div style="display: inline-block; width: 72px; text-align: center; vertical-align: top; padding: 4px 0;">
    <a href="{{ .CSATLink }}?rating=5" style="text-decoration: none; display: block;">
      <span style="font-size: 34px; display: block; line-height: 1.4;">&#129321;</span>
      <span style="font-size: 10px; display: block; font-weight: 600; color: #b0b5bd; text-transform: uppercase; letter-spacing: 0.05em; margin-top: 4px;">Excellent</span>
    </a>
  </div>
</div>
',
  false,
  'CSAT request',
  '',
  true
);

-- Default business hours
INSERT INTO business_hours (tenant_id, "name", description, is_always_open, hours, holidays) VALUES
(p_tenant_id, 'Default', 'Default business hours, Monday to Friday, 09:00 to 17:00.', false, '{"Monday": {"open": "09:00", "close": "17:00"}, "Tuesday": {"open": "09:00", "close": "17:00"}, "Wednesday": {"open": "09:00", "close": "17:00"}, "Thursday": {"open": "09:00", "close": "17:00"}, "Friday": {"open": "09:00", "close": "17:00"}}'::jsonb, '[]'::jsonb);

-- Default SLA policy
INSERT INTO sla_policies (tenant_id, "name", description, first_response_time, resolution_time, next_response_time, notifications) VALUES
(p_tenant_id, 'Default', 'Default SLA policy, first response within 1 hour and resolution within 24 hours.', '1h', '24h', NULL, '[]'::jsonb);
END;
$seed$ LANGUAGE plpgsql;
