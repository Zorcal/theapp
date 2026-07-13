// Package auth provides the DBOS workflow layer for magic-link authentication.
package auth

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	htmltmpl "html/template"
	texttmpl "text/template"

	"github.com/zorcal/theapp/backend/internal/core/auth"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/email"
	"github.com/zorcal/theapp/backend/internal/workflows"
)

//go:embed templates/magic_link_email.txt templates/magic_link_email.html
var emailFS embed.FS

var (
	magicLinkEmailTextTmpl = texttmpl.Must(texttmpl.ParseFS(emailFS, "templates/magic_link_email.txt"))
	magicLinkEmailHTMLTmpl = htmltmpl.Must(htmltmpl.ParseFS(emailFS, "templates/magic_link_email.html"))
)

type magicLinkTemplateData struct {
	Link string
	TTL  string
}

//go:generate moq -rm -fmt goimports -out authcore_moq_test.go . AuthCore:MockedAuthCore

// AuthCore is the subset of non-durable auth operations WorkflowCore depends on.
// Implemented by *core/auth.Core.
type AuthCore interface {
	// MagicLinkToken ensures the user exists, rate-checks, invalidates prior tokens, and creates a new one.
	// Returns [mdl.ErrRateLimited] if a token was already issued to rml.Email within the rate-limit window.
	// Returns [mdl.ErrValidation] if rml is invalid.
	MagicLinkToken(ctx context.Context, rml mdl.RequestMagicLink) (string, error)
}

// WorkflowCore executes auth operations that require durable execution via DBOS.
type WorkflowCore struct {
	core        AuthCore
	emailSender email.Sender
	cfg         auth.Config
	dbosCtx     dbos.DBOSContext
}

// NewWorkflowCore constructs a WorkflowCore.
func NewWorkflowCore(core AuthCore, emailSender email.Sender, cfg auth.Config, dbosCtx dbos.DBOSContext) *WorkflowCore {
	return &WorkflowCore{
		core:        core,
		emailSender: emailSender,
		cfg:         cfg,
		dbosCtx:     dbosCtx,
	}
}

// RegisterWorkflows registers all DBOS workflow functions. Must be called before dbos.Launch.
func RegisterWorkflows(ctx dbos.DBOSContext, wc *WorkflowCore) {
	dbos.RegisterWorkflow(ctx, wc.requestMagicLinkWorkflow)
}

// RequestMagicLink sends a sign-in link to emailAddr durably. If the process crashes after the token is stored but
// before the email is sent, DBOS resumes from the email step on restart. If ctx carries a workflow ID, DBOS
// deduplicates on it so retrying with the same key returns the original result without sending a second email.
// Returns nil without sending an email if emailAddr is rate-limited.
func (w *WorkflowCore) RequestMagicLink(ctx context.Context, emailAddr string) error {
	opts := []dbos.WorkflowOption{}
	if id := workflows.WorkflowID(ctx); id != "" {
		opts = append(opts, dbos.WithWorkflowID(id))
	}

	handle, err := dbos.RunWorkflow(dbos.From(w.dbosCtx, ctx), w.requestMagicLinkWorkflow, emailAddr, opts...)
	if err != nil {
		return fmt.Errorf("run workflow: %w", err)
	}

	if _, err := handle.GetResult(); err != nil {
		return fmt.Errorf("wait for workflow completion: %w", err)
	}

	return nil
}

func (w *WorkflowCore) requestMagicLinkWorkflow(ctx dbos.DBOSContext, emailAddr string) (struct{}, error) {
	rawToken, err := dbos.RunAsStep(ctx, w.storeTokenStep(emailAddr), dbos.WithStepName("store-token"))
	if err != nil {
		if errors.Is(err, mdl.ErrRateLimited) {
			return struct{}{}, nil
		}
		return struct{}{}, fmt.Errorf("run step: generate magic link token: %w", err)
	}

	if _, err = dbos.RunAsStep(ctx, w.sendEmailStep(emailAddr, rawToken), dbos.WithStepName("send-email")); err != nil {
		return struct{}{}, fmt.Errorf("run step: send email: %w", err)
	}

	return struct{}{}, nil
}

func (w *WorkflowCore) storeTokenStep(emailAddr string) dbos.Step[string] {
	return func(ctx context.Context) (string, error) {
		return w.core.MagicLinkToken(ctx, mdl.RequestMagicLink{Email: emailAddr})
	}
}

func (w *WorkflowCore) sendEmailStep(emailAddr, rawToken string) dbos.Step[struct{}] {
	return func(ctx context.Context) (struct{}, error) {
		return struct{}{}, w.sendMagicLinkEmail(ctx, emailAddr, rawToken)
	}
}

func (w *WorkflowCore) sendMagicLinkEmail(ctx context.Context, emailAddr, rawToken string) error {
	link := w.cfg.MagicLinkBaseURL + "?token=" + rawToken
	tmplData := magicLinkTemplateData{
		Link: link,
		TTL:  w.cfg.MagicLinkTTL.String(),
	}

	// Execute errors are unreachable: both templates only reference the Link and
	// TTL fields which magicLinkTemplateData always provides.
	var textBuf, htmlBuf strings.Builder
	if err := magicLinkEmailTextTmpl.Execute(&textBuf, tmplData); err != nil {
		return fmt.Errorf("render magic link text email: %w", err)
	}
	if err := magicLinkEmailHTMLTmpl.Execute(&htmlBuf, tmplData); err != nil {
		return fmt.Errorf("render magic link html email: %w", err)
	}

	if err := w.emailSender.SendEmail(ctx, email.Message{
		From:     w.cfg.MagicLinkFromEmail,
		To:       []string{emailAddr},
		Subject:  "Your sign-in link",
		TextBody: textBuf.String(),
		HTMLBody: htmlBuf.String(),
	}); err != nil {
		return fmt.Errorf("send magic link email: %w", err)
	}

	return nil
}
