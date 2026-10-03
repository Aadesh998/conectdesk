package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/valyala/fasthttp"
)

const (
	// Built-in templates names stored in the database.
	TmplConversationAssigned = "Conversation assigned"
	TmplSLABreachWarning     = "SLA breach warning"
	TmplSLABreached          = "SLA breached"
	TmplMentioned            = "Mentioned in conversation"
	TmplCSATRequest          = "CSAT request"

	// Built-in templates fetched from memory stored in `static` directory.
	TmplResetPassword = "reset-password"
	TmplWelcome       = "welcome"

	// Template names for rendering.
	TmplBase    = "base"
	TmplContent = "content"
)

// RenderString renders Go template variables in the given content string
// without wrapping it in the base email template. Returns original content on any error.
func (m *Manager) RenderString(tenantID int, data any, content string) string {
	t, err := template.New("content").Funcs(m.funcsFor(tenantID)).Parse(content)
	if err != nil {
		return content
	}
	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return content
	}
	return buf.String()
}

// RenderStoredTemplate fetches a template by name and renders its body with the provided data
// without wrapping it in the base email template.
func (m *Manager) RenderStoredTemplate(tenantID int, name string, data any) (string, error) {
	tmpl, err := m.getByName(tenantID, name)
	if err != nil {
		return "", err
	}
	return m.RenderString(tenantID, data, tmpl.Body), nil
}

// RenderEmailWithTemplate renders content inside the default outgoing email template.
func (m *Manager) RenderEmailWithTemplate(tenantID int, data any, content string) (string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	funcs := m.funcsFor(tenantID)

	defaultTmpl, err := m.getDefaultOutgoingEmailTemplate(tenantID)
	if err != nil {
		m.lo.Error("error fetching default outgoing email template", "error", err)
	}

	if defaultTmpl.Body == "" {
		defaultTmpl.Body = `{{ template "content" . }}`
	}

	baseTemplate, err := template.New(TmplBase).Funcs(funcs).Parse(defaultTmpl.Body)
	if err != nil {
		return "", fmt.Errorf("parsing base template: %w", err)
	}

	contentTemplate, err := template.New(TmplContent).Funcs(funcs).Parse(content)
	if err != nil {
		return "", fmt.Errorf("parsing content template: %w", err)
	}

	baseTemplate, err = baseTemplate.AddParseTree(TmplContent, contentTemplate.Tree)
	if err != nil {
		return "", fmt.Errorf("adding content template: %w", err)
	}

	var rendered strings.Builder
	if err := baseTemplate.ExecuteTemplate(&rendered, TmplBase, data); err != nil {
		return "", fmt.Errorf("executing base template: %w", err)
	}

	return rendered.String(), nil
}

// RenderStoredEmailTemplate fetches and renders an email template from the database, including subject and body and returns the rendered content.
func (m *Manager) RenderStoredEmailTemplate(tenantID int, name string, data any) (string, string, error) {
	funcs := m.funcsFor(tenantID)
	tmpl, err := m.getByName(tenantID, name)
	if err != nil {
		if err == ErrTemplateNotFound {
			return "", "", fmt.Errorf("template %s not found", name)
		}
		return "", "", err
	}

	executeSubjectTemplate := func(subject string) (string, error) {
		var sb strings.Builder
		subjectTmpl, err := template.New("subject").Funcs(funcs).Parse(subject)
		if err != nil {
			return "", fmt.Errorf("parsing subject template: %w", err)
		}
		if err := subjectTmpl.Execute(&sb, data); err != nil {
			return "", fmt.Errorf("executing subject template: %w", err)
		}
		return sb.String(), nil
	}

	defaultTmpl, err := m.getDefaultOutgoingEmailTemplate(tenantID)
	if err != nil {
		m.lo.Error("error fetching default outgoing email template", "error", err)
	}

	if defaultTmpl.Body == "" {
		defaultTmpl.Body = `{{ template "content" . }}`
	}

	baseTemplate, err := template.New(TmplBase).Funcs(funcs).Parse(defaultTmpl.Body)
	if err != nil {
		return "", "", fmt.Errorf("parsing base template: %w", err)
	}

	contentTemplate, err := template.New(TmplContent).Funcs(funcs).Parse(tmpl.Body)
	if err != nil {
		return "", "", fmt.Errorf("parsing content template: %w", err)
	}

	baseTemplate, err = baseTemplate.AddParseTree(TmplContent, contentTemplate.Tree)
	if err != nil {
		return "", "", fmt.Errorf("adding content template: %w", err)
	}

	var rendered strings.Builder
	if err := baseTemplate.ExecuteTemplate(&rendered, TmplBase, data); err != nil {
		return "", "", fmt.Errorf("executing base template: %w", err)
	}

	subject, err := executeSubjectTemplate(tmpl.Subject.String)
	if err != nil {
		return "", "", err
	}

	return rendered.String(), subject, nil
}

// RenderInMemoryTemplate executes an in-memory template with data and returns the rendered content.
// This is for system emails like reset password and welcome email etc.
func (m *Manager) RenderInMemoryTemplate(tenantID int, name string, data interface{}) (string, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	tpls, err := m.setFor(tenantID, false)
	if err != nil {
		return "", fmt.Errorf("preparing templates: %w", err)
	}
	var buf bytes.Buffer
	if err := tpls.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("executing in-memory template %q: %w", name, err)
	}
	return buf.String(), nil
}

// RenderWebPage renders a template to the http.ResponseWriter with data.
func (m *Manager) RenderWebPage(tenantID int, ctx *fasthttp.RequestCtx, tmplFile string, data map[string]interface{}) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	webTpls, err := m.setFor(tenantID, true)
	if err != nil {
		return fmt.Errorf("preparing templates: %w", err)
	}
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusOK)
	// Add no-cache headers
	ctx.Response.Header.Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Response.Header.Set("Pragma", "no-cache")
	ctx.Response.Header.Set("Expires", "0")
	return webTpls.ExecuteTemplate(ctx, tmplFile, data)
}
