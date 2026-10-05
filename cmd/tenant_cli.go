package main

import (
	"bufio"
	"cmp"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/conectdesk/conectdesk/internal/demo"
	"github.com/conectdesk/conectdesk/internal/platform"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/user"
	"github.com/conectdesk/conectdesk/internal/workspace"
	"github.com/jmoiron/sqlx"
	"github.com/knadh/stuffbin"
	"golang.org/x/term"
)

// runTenantCommand runs a --tenant-* platform admin command, reporting whether
// one was given.
func runTenantCommand(db *sqlx.DB, fs stuffbin.FileSystem) bool {
	var (
		list     = ko.Bool("tenant-list")
		suspend  = ko.String("tenant-suspend")
		activate = ko.String("tenant-activate")
		del      = ko.String("tenant-delete")
		restore  = ko.String("tenant-restore")
		seed     = ko.String("seed-demo")
		create   = ko.String("tenant-create")
	)
	if !list && suspend == "" && activate == "" && del == "" && restore == "" && seed == "" && create == "" {
		return false
	}

	mgr := initTenant(db)
	switch {
	case create != "":
		createTenantCommand(db, mgr, create)
	case seed != "":
		t := mustTenant(mgr, seed)
		sum, err := demo.Seed(db, t.ID, time.Now())
		if err != nil {
			log.Fatalf("error adding demo data to %q: %v", t.Slug, err)
		}
		fmt.Printf("added demo data to %q: %d contacts, %d conversations, %d call-backs\n", t.Slug, sum.Contacts, sum.Conversations, sum.CallBacks)
	case list:
		tenants, err := mgr.GetAll()
		if err != nil {
			log.Fatalf("error listing tenants: %v", err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSLUG\tNAME\tPLAN\tSTATUS\tCREATED\tUSAGE")
		for _, t := range tenants {
			usage, _ := mgr.Usage(t.ID)
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t(%d agents, %d inboxes)\n", t.ID, t.Slug, t.Name, t.Plan, t.Status,
				t.CreatedAt.Format("2006-01-02"), usage.Agents, usage.Inboxes)
		}
		w.Flush()
	case restore != "":
		t := mustTenant(mgr, restore)
		if _, err := mgr.CancelDeletion(t.ID); err != nil {
			log.Fatalf("tenant %q isn't scheduled for deletion", t.Slug)
		}
		fmt.Printf("restored tenant %q\n", t.Slug)
	case suspend != "":
		setTenantStatus(mgr, suspend, tmodels.StatusSuspended)
	case activate != "":
		setTenantStatus(mgr, activate, tmodels.StatusActive)
	case del != "":
		t := mustTenant(mgr, del)
		if t.ID == tenant.DefaultID {
			log.Fatalf("the default tenant can't be deleted")
		}
		if !ko.Bool("yes") {
			fmt.Printf("Delete tenant %q (%s) and ALL its data? This can't be undone. Type the slug to confirm: ", t.Slug, t.Name)
			var answer string
			fmt.Scanln(&answer)
			if strings.TrimSpace(answer) != t.Slug {
				log.Fatalf("deletion cancelled")
			}
		}
		// The tenant's stored files go too, not only its rows.
		med := initMedia(db, initI18n(fs), func(int) string { return "" })
		if err := deleteTenantData(mgr, med, t.ID); err != nil {
			log.Fatalf("error deleting tenant: %v", err)
		}
		fmt.Printf("deleted tenant %q\n", t.Slug)
	}
	return true
}

// createTenantCommand creates a workspace with its admin, e.g. a demo
// workspace. The admin's password is read from the terminal, or from stdin.
func createTenantCommand(db *sqlx.DB, mgr *tenant.Manager, slug string) {
	if !mgr.Enabled() {
		log.Fatalf("workspaces can only be created with multi-tenancy on (app.tenancy.enabled)")
	}
	first, last, _ := strings.Cut(strings.TrimSpace(ko.String("tenant-admin-name")), " ")
	req := workspace.Request{
		Company:   ko.String("tenant-name"),
		Slug:      slug,
		FirstName: first,
		LastName:  last,
		Email:     ko.String("tenant-admin-email"),
		Password:  readPassword("Password for " + ko.String("tenant-admin-email") + ": "),
	}
	m := workspace.New(workspace.Opts{DB: db, Lo: initLogger("workspace"), Tenants: mgr, Plan: cmp.Or(ko.String("app.tenancy.default_plan"), "free")})
	t, err := m.Create(context.Background(), req)
	if err != nil {
		log.Fatalf("error creating workspace %q: %v", slug, err)
	}
	fmt.Printf("created workspace %q (%s) at %s, with admin %s\n", t.Slug, t.Name, mgr.URLFor(t), req.Email)
}

func mustTenant(mgr *tenant.Manager, slug string) tmodels.Tenant {
	t, err := mgr.GetBySlug(slug)
	if err != nil {
		log.Fatalf("tenant %q: %v", slug, err)
	}
	return t
}

func setTenantStatus(mgr *tenant.Manager, slug, status string) {
	t := mustTenant(mgr, slug)
	if t.ID == tenant.DefaultID && status == tmodels.StatusSuspended {
		log.Fatalf("the default tenant can't be suspended")
	}
	if _, err := mgr.SetStatus(t.ID, status); err != nil {
		log.Fatalf("error updating tenant: %v", err)
	}
	fmt.Printf("tenant %q is now %s\n", t.Slug, status)
}

// runPlatformAdminCommand runs a --platform-admin-* command, reporting whether
// one was given. The password is read from the terminal without echo, or from
// stdin when it isn't a terminal.
func runPlatformAdminCommand(db *sqlx.DB) bool {
	var (
		add    = ko.String("platform-admin-add")
		remove = ko.String("platform-admin-remove")
		list   = ko.Bool("platform-admin-list")
		reset  = ko.String("platform-admin-reset-2fa")
	)
	if add == "" && remove == "" && !list && reset == "" {
		return false
	}
	mgr, err := platform.New(platform.Opts{DB: db, Lo: initLogger("platform"), EncryptionKey: ko.MustString("app.encryption_key")})
	if err != nil {
		log.Fatalf("error initializing platform dashboard: %v", err)
	}

	switch {
	case list:
		admins, err := mgr.Admins()
		if err != nil {
			log.Fatalf("error listing platform admins: %v", err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tCREATED\tLAST LOGIN\t2FA")
		for _, a := range admins {
			last := "never"
			if a.LastLoginAt.Valid {
				last = a.LastLoginAt.Time.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%v\n", a.ID, a.Email, a.CreatedAt.Format("2006-01-02"), last, a.TOTPEnabled)
		}
		w.Flush()
	case reset != "":
		ok, err := mgr.ResetTwoFactor(reset)
		if err != nil {
			log.Fatalf("error resetting two-factor login: %v", err)
		}
		if !ok {
			log.Fatalf("no platform admin %q", reset)
		}
		fmt.Printf("two-factor login of %q is off; they set it up again at their next login\n", reset)
	case remove != "":
		ok, err := mgr.RemoveAdmin(remove)
		if err != nil {
			log.Fatalf("error removing platform admin: %v", err)
		}
		if !ok {
			log.Fatalf("no platform admin %q", remove)
		}
		fmt.Printf("removed platform admin %q\n", remove)
	case add != "":
		password := readPassword("Password for " + add + ": ")
		if !user.IsStrongPassword(password) {
			log.Fatalf("the password needs 10 to 72 characters with an uppercase letter, a lowercase letter, a number and a symbol")
		}
		if term.IsTerminal(int(os.Stdin.Fd())) && readPassword("Repeat it: ") != password {
			log.Fatalf("the passwords don't match")
		}
		a, err := mgr.SetAdmin(add, password)
		if err != nil {
			log.Fatalf("error saving platform admin: %v", err)
		}
		fmt.Printf("platform admin %q can log in at the platform dashboard\n", a.Email)
	}
	return true
}

// runDocsViewerCommand runs a --docs-viewer-* command, reporting whether one
// was given. Docs viewers can log in to the internal documentation only. The
// password is read as for --platform-admin-add.
func runDocsViewerCommand(db *sqlx.DB) bool {
	var (
		add    = ko.String("docs-viewer-add")
		remove = ko.String("docs-viewer-remove")
		list   = ko.Bool("docs-viewer-list")
	)
	if add == "" && remove == "" && !list {
		return false
	}
	mgr, err := platform.New(platform.Opts{DB: db, Lo: initLogger("platform"), EncryptionKey: ko.MustString("app.encryption_key")})
	if err != nil {
		log.Fatalf("error initializing platform dashboard: %v", err)
	}

	switch {
	case list:
		viewers, err := mgr.DocsViewers()
		if err != nil {
			log.Fatalf("error listing docs viewers: %v", err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tEMAIL\tCREATED\tLAST LOGIN")
		for _, v := range viewers {
			last := "never"
			if v.LastLoginAt.Valid {
				last = v.LastLoginAt.Time.Format("2006-01-02 15:04")
			}
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", v.ID, v.Email, v.CreatedAt.Format("2006-01-02"), last)
		}
		w.Flush()
	case remove != "":
		ok, err := mgr.RemoveDocsViewer(remove)
		if err != nil {
			log.Fatalf("error removing docs viewer: %v", err)
		}
		if !ok {
			log.Fatalf("no docs viewer %q", remove)
		}
		fmt.Printf("removed docs viewer %q\n", remove)
	case add != "":
		password := readPassword("Password for " + add + ": ")
		if !user.IsStrongPassword(password) {
			log.Fatalf("the password needs 10 to 72 characters with an uppercase letter, a lowercase letter, a number and a symbol")
		}
		if term.IsTerminal(int(os.Stdin.Fd())) && readPassword("Repeat it: ") != password {
			log.Fatalf("the passwords don't match")
		}
		v, err := mgr.SetDocsViewer(add, password)
		if err != nil {
			log.Fatalf("error saving docs viewer: %v", err)
		}
		fmt.Printf("docs viewer %q can log in to the documentation\n", v.Email)
	}
	return true
}

func readPassword(prompt string) string {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		log.Fatalf("error reading password: %v", err)
	}
	return string(b)
}
