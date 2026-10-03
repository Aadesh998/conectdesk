package main

import (
	"errors"
	"strconv"
	"strings"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// tlsAskPath answers whether the installation serves a hostname, for TLS
// servers issuing certificates on demand, e.g. Caddy's on_demand_tls ask.
const tlsAskPath = "/tls/ask"

// domainResp is a custom domain with the DNS records that set it up.
type domainResp struct {
	tmodels.Domain
	Verified bool `json:"verified"`
	// RecordName and RecordValue are the TXT record that verifies the domain.
	RecordName  string `json:"record_name"`
	RecordValue string `json:"record_value"`
}

// domainsResp is the tenant's custom domains and where they point.
type domainsResp struct {
	// Address is the workspace's URL, its primary domain's or its subdomain's.
	Address string `json:"address"`
	// Target is the workspace subdomain, which custom domains' CNAME records point at.
	Target  string       `json:"target"`
	Domains []domainResp `json:"domains"`
}

func newDomainResp(d tmodels.Domain) domainResp {
	name, value := tenant.VerificationRecord(d)
	return domainResp{Domain: d, Verified: d.Verified(), RecordName: name, RecordValue: value}
}

// handleGetDomains returns the tenant's custom domains.
func handleGetDomains(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.tenant.Enabled() {
		return sendDomainError(r, errDomainsNeedTenancy)
	}
	domains, err := app.tenant.Domains(tenantID(r))
	if err != nil {
		return sendDomainError(r, err)
	}
	address, err := app.tenant.RootURL(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	out := domainsResp{
		Address: address,
		Target:  tenantFromReq(r).Slug + "." + app.tenant.BaseDomain(),
		Domains: make([]domainResp, 0, len(domains)),
	}
	for _, d := range domains {
		out.Domains = append(out.Domains, newDomainResp(d))
	}
	return r.SendEnvelope(out)
}

// handleCreateDomain adds a custom domain, unverified.
func handleCreateDomain(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			Hostname string `json:"hostname"`
		}
	)
	if !app.tenant.Enabled() {
		return sendDomainError(r, errDomainsNeedTenancy)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	d, err := app.tenant.AddDomain(tenantID(r), req.Hostname)
	if err != nil {
		return sendDomainError(r, err)
	}
	return r.SendEnvelope(newDomainResp(d))
}

// handleVerifyDomain checks the domain's DNS record and marks it verified when
// it matches. A record that doesn't match isn't an error; the domain comes back
// unverified with the reason in last_error.
func handleVerifyDomain(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.tenant.Enabled() {
		return sendDomainError(r, errDomainsNeedTenancy)
	}
	id, ok := domainID(r)
	if !ok {
		return sendDomainError(r, tenant.ErrDomainNotFound)
	}
	d, err := app.tenant.VerifyDomain(r.RequestCtx, tenantID(r), id)
	if err != nil {
		return sendDomainError(r, err)
	}
	return r.SendEnvelope(newDomainResp(d))
}

// handleSetPrimaryDomain makes a verified domain the workspace address, or the
// subdomain again for domain_id 0.
func handleSetPrimaryDomain(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		req struct {
			DomainID int `json:"domain_id"`
		}
	)
	if !app.tenant.Enabled() {
		return sendDomainError(r, errDomainsNeedTenancy)
	}
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tenantID(r)).T("errors.parsingRequest"), err.Error(), envelope.InputError)
	}
	if err := app.tenant.SetPrimaryDomain(tenantID(r), req.DomainID); err != nil {
		return sendDomainError(r, err)
	}
	// Links and emails use the workspace address from its constants.
	app.tenantConsts.Delete(tenantID(r))
	return handleGetDomains(r)
}

// handleDeleteDomain removes a custom domain.
func handleDeleteDomain(r *fastglue.Request) error {
	app := r.Context.(*App)
	if !app.tenant.Enabled() {
		return sendDomainError(r, errDomainsNeedTenancy)
	}
	id, ok := domainID(r)
	if !ok {
		return sendDomainError(r, tenant.ErrDomainNotFound)
	}
	if err := app.tenant.RemoveDomain(tenantID(r), id); err != nil {
		return sendDomainError(r, err)
	}
	app.tenantConsts.Delete(tenantID(r))
	return r.SendEnvelope(true)
}

var errDomainsNeedTenancy = errors.New("custom domains need multi-tenancy")

// sendDomainError sends the domain error as a translated envelope.
func sendDomainError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	var (
		code = fasthttp.StatusBadRequest
		key  string
		typ  fastglue.ErrorType = envelope.InputError
	)
	switch {
	case errors.Is(err, tenant.ErrInvalidDomain):
		key = "domain.invalid"
	case errors.Is(err, tenant.ErrDomainExists):
		code, key, typ = fasthttp.StatusConflict, "domain.exists", envelope.ConflictError
	case errors.Is(err, tenant.ErrDomainTaken):
		code, key, typ = fasthttp.StatusConflict, "domain.taken", envelope.ConflictError
	case errors.Is(err, tenant.ErrDomainNotFound):
		code, key, typ = fasthttp.StatusNotFound, "domain.notFound", envelope.NotFoundError
	case errors.Is(err, tenant.ErrDomainNotVerified):
		key = "domain.notVerified"
	case errors.Is(err, tenant.ErrDomainInUse):
		code, key, typ = fasthttp.StatusConflict, "domain.inUse", envelope.ConflictError
	case errors.Is(err, errDomainsNeedTenancy):
		code, key, typ = fasthttp.StatusNotFound, "domain.needsTenancy", envelope.NotFoundError
	default:
		return sendErrorEnvelope(r, err)
	}
	return r.SendErrorEnvelope(code, app.i18n.For(tenantID(r)).T(key), nil, typ)
}

func domainID(r *fastglue.Request) (int, bool) {
	id, err := strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	return id, err == nil && id > 0
}

// serveTLSAsk answers 200 when the installation serves the "domain" query
// parameter's hostname and 404 otherwise, so a TLS server only requests
// certificates for its own and verified domains.
func serveTLSAsk(app *App, ctx *fasthttp.RequestCtx) {
	host := strings.TrimSpace(string(ctx.QueryArgs().Peek("domain")))
	ok, err := app.tenant.ServesHost(host)
	switch {
	case err != nil:
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
	case ok && host != "":
		ctx.SetStatusCode(fasthttp.StatusOK)
	default:
		ctx.SetStatusCode(fasthttp.StatusNotFound)
	}
}
