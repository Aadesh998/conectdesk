# Signing in and your account

How agents get into a workspace at `https://<slug>.desk.arrlink.com` (or the workspace's own domain), and what they can change about their own account. Anyone who forgets their workspace address can find it on the landing page at `https://desk.arrlink.com` (see Platform admin).

## Login screen

**Where:** `/` on the workspace address. Anyone who isn't logged in lands here.

**Who can use it:** Anyone.

**Purpose:** Sign in with email and password, or with a single-sign-on (SSO/OIDC) provider.

| Control | What it does | Typical use |
|---|---|---|
| Workspace name heading + "Sign in to your account" | Shows the site name (defaults to "ConnectDesk"). | — |
| One button per SSO provider (provider name and logo) | Sends you to that provider's login. Only enabled providers are shown. | Companies using Google, Okta, etc. |
| "Or continue with" divider | Separates the SSO buttons from the password form. | — |
| **Email** | Your login email. Must be a valid email (the special user "System" is the only exception). | — |
| **Password** + eye icon ("Show password" / "Hide password") | Enter the password; the eye shows or hides it. | — |
| **Forgot password?** | Opens the Reset password screen. | Locked out. |
| **Sign in** ("Logging in..." while working) | Checks your credentials. On success you go to My Inbox, or to the page you were trying to reach (`?next=`). | — |
| Error box (the card shakes) | Shows "Invalid email address", "Password cannot be empty" or the server's error. | — |

**SSO-required workspaces:** when the workspace requires SSO and at least one provider exists, the password form is hidden. A link **Admin? Log in with your password** reveals it, so a broken provider can't lock admins out.

**SSO errors shown on return:**

- "Sign-in is unavailable due to a configuration problem…"
- "The sign-in was cancelled or denied…"
- "Your sign-in session expired…"
- "There is no agent account for the email…"
- "Your account is disabled, Please contact administrator"
- "Sign-in failed…"

Demo builds pre-fill demo@conectdesk.io.

## Two-factor code step

**Where:** Same screen. It appears after a correct password when the account has two-factor login turned on.

**Who can use it:** Users with two-factor login on.

| Control | What it does |
|---|---|
| **Two-factor code** | Prompt: "Enter the 6-digit code from your authenticator app, or one of your recovery codes." Up to 12 characters. |
| **Verify** | Checks the code. If the challenge has expired, you go back to the password form. |
| **Back to login** | Cancels and returns to the email/password form. |

## Reset password

**Where:** Login → **Forgot password?** (`/reset-password`).

**Who can use it:** Anyone.

**Purpose:** Email yourself a link to reset your password.

| Control | What it does |
|---|---|
| **Email** | Your account email. |
| **Send Reset Link** ("Sending..." while working) | Requests the reset email ("Check your email for the password reset link."), then returns to login. |
| **Back to login** | Returns to `/`. |

## Set new password

**Where:** The link in the reset or invite email: `/set-password?token=…`. Without a token you are sent to login.

**Who can use it:** Anyone holding a valid link.

| Control | What it does |
|---|---|
| **New password** + eye toggle | Must be within the minimum and maximum length ("Must be between {min} and {max} characters"). |
| **Confirm password** + eye toggle | Must match ("Passwords do not match."). |
| **Set new password** ("Setting password..." while working) | Saves the password, shows "You can now login with your new password." and returns to login. |

## Account → Profile

**Where:** Click your avatar (bottom of the left icon rail) → **Account** (`/account/profile`). The account sidebar shows **Profile** and **Security**.

**Who can use it:** Every user.

| Control | What it does |
|---|---|
| **Your details**: First name, Last name | Your name as teammates and customers see it. |
| **Phone for calls** | Your number with country code. Exotel rings it for phone-channel calls. |
| Read-only details | Email, Workspace, Roles, Teams. "To change your email or roles, ask an admin of your workspace." |
| **Save changes** | Saves name and phone. Enabled only when something changed. |
| **Browser calling**: "Take calls in this browser" switch | Calls ring in the browser instead of your phone. The status line says e.g. "Connected. Calls ring here.", "Another tab of this browser is taking your calls." or "The microphone is blocked…". Shown only after an admin sets up your SIP account (Admin → Agents). |
| **Public avatar**: upload / crop / × remove, then **Save changes** | Changes your picture. Removing it takes effect immediately ("Avatar removed"). |

## Account → Security (two-factor login)

**Where:** `/account/security`. You are forced here if the workspace requires two-factor login and you haven't set it up: "Your workspace requires two-factor login. Set it up to continue."

**Who can use it:** Every user.

| State | Controls |
|---|---|
| Off | **Set up two-factor login**: shows a QR code and the **Key**. Scan it with an authenticator app (Google Authenticator, 1Password, Authy…), enter "The 6-digit code the app shows", then **Turn on**. |
| Just turned on | **Recovery codes** list, shown only once. **Copy** / **I've saved them**. Each code logs you in once if you lose your phone. |
| On | "On since {date} · {n} recovery codes left". Enter **Your password**, then **New recovery codes** (replaces the old ones) or **Turn off** (hidden if the workspace requires two-factor login). |

If an agent loses their phone and their recovery codes, a workspace admin can reset their two-factor login under Admin → Agents.

## Known quirk

When a session expires the app sends you to `/?redirect=<page>`, but the login screen only reads `?next=`. After a timeout, users therefore land on My Inbox rather than the page they were on.
