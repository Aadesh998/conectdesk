package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Health is a number's standing with Meta, which decides how many people it
// may message and whether it may send at all.
type Health struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
	// QualityRating is GREEN, YELLOW, RED or UNKNOWN, from how recipients
	// received the number's recent messages.
	QualityRating string `json:"quality_rating"`
	// MessagingLimitTier is how many people the number may start conversations
	// with a day, e.g. TIER_250, TIER_1K, TIER_UNLIMITED.
	MessagingLimitTier string `json:"messaging_limit_tier"`
	// Status is e.g. CONNECTED, FLAGGED, RESTRICTED or DISCONNECTED.
	Status string `json:"status"`
	// NameStatus is the review of the display name, e.g. APPROVED, DECLINED.
	NameStatus string `json:"name_status"`
	Throughput struct {
		Level string `json:"level"`
	} `json:"throughput"`
}

// Health reads the number's current standing from the Graph API.
func (w *WhatsApp) Health(ctx context.Context) (Health, error) {
	var h Health
	q := url.Values{"fields": {"display_phone_number,verified_name,quality_rating,messaging_limit_tier,status,name_status,throughput"}}
	err := w.graph.Get(ctx, w.config.AccessToken, w.config.AccountID, q, &h)
	return h, err
}

// Alert severities.
const (
	AlertInfo     = "info"
	AlertWarning  = "warning"
	AlertCritical = "critical"
)

// HealthAlert is the last thing Meta reported about a number's standing, kept
// on the inbox config under "health_alert" until the next report.
type HealthAlert struct {
	Event    string    `json:"event"`
	Severity string    `json:"severity"`
	Detail   string    `json:"detail"`
	At       time.Time `json:"at"`
}

// QualityAlert turns a phone_number_quality_update change into an alert.
func QualityAlert(value json.RawMessage) (displayNumber string, alert HealthAlert, ok bool) {
	var v struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		Event              string `json:"event"`
		CurrentLimit       string `json:"current_limit"`
		OldLimit           string `json:"old_limit"`
	}
	if json.Unmarshal(value, &v) != nil || v.Event == "" {
		return "", HealthAlert{}, false
	}
	alert = HealthAlert{Event: v.Event, At: time.Now().UTC()}
	switch v.Event {
	case "FLAGGED":
		alert.Severity = AlertWarning
		alert.Detail = "Quality has dropped to low. If it doesn't improve within 7 days, the messaging limit goes down."
	case "DOWNGRADE":
		alert.Severity = AlertCritical
		alert.Detail = "The messaging limit went down"
	case "UNFLAGGED":
		alert.Severity = AlertInfo
		alert.Detail = "Quality has recovered."
	case "UPGRADE", "ONBOARDING":
		alert.Severity = AlertInfo
		alert.Detail = "The messaging limit went up"
	default:
		alert.Severity = AlertInfo
		alert.Detail = strings.ToLower(strings.ReplaceAll(v.Event, "_", " "))
	}
	if v.CurrentLimit != "" && (v.Event == "DOWNGRADE" || v.Event == "UPGRADE" || v.Event == "ONBOARDING") {
		alert.Detail += " to " + LimitLabel(v.CurrentLimit) + "."
	}
	return WAIDFromPhone(v.DisplayPhoneNumber), alert, true
}

// AccountAlert turns an account_update change into an alert, for the events
// that affect sending. PARTNER_REMOVED is handled separately.
func AccountAlert(value json.RawMessage) (HealthAlert, bool) {
	var v struct {
		Event           string `json:"event"`
		RestrictionInfo []struct {
			RestrictionType string `json:"restriction_type"`
			Expiration      string `json:"expiration"`
		} `json:"restriction_info"`
		ViolationInfo struct {
			ViolationType string `json:"violation_type"`
		} `json:"violation_info"`
		BanInfo struct {
			State string `json:"waba_ban_state"`
		} `json:"ban_info"`
	}
	if json.Unmarshal(value, &v) != nil {
		return HealthAlert{}, false
	}
	alert := HealthAlert{Event: v.Event, At: time.Now().UTC()}
	switch v.Event {
	case "ACCOUNT_RESTRICTION":
		alert.Severity = AlertCritical
		var parts []string
		for _, r := range v.RestrictionInfo {
			p := strings.ToLower(strings.ReplaceAll(r.RestrictionType, "_", " "))
			if r.Expiration != "" {
				p += " until " + r.Expiration
			}
			parts = append(parts, p)
		}
		alert.Detail = "Meta restricted the account: " + strings.Join(parts, "; ")
	case "ACCOUNT_VIOLATION":
		alert.Severity = AlertWarning
		alert.Detail = "Meta found a policy violation: " + strings.ToLower(strings.ReplaceAll(v.ViolationInfo.ViolationType, "_", " "))
	case "DISABLED_UPDATE":
		alert.Severity = AlertCritical
		alert.Detail = "Meta disabled the account (" + strings.ToLower(v.BanInfo.State) + ")"
	default:
		return HealthAlert{}, false
	}
	return alert, true
}

// NameAlert turns a phone_number_name_update change into an alert.
func NameAlert(value json.RawMessage) (displayNumber string, alert HealthAlert, ok bool) {
	var v struct {
		DisplayPhoneNumber string `json:"display_phone_number"`
		Decision           string `json:"decision"`
		RequestedName      string `json:"requested_verified_name"`
		RejectionReason    string `json:"rejection_reason"`
	}
	if json.Unmarshal(value, &v) != nil || v.Decision == "" {
		return "", HealthAlert{}, false
	}
	alert = HealthAlert{Event: "NAME_" + v.Decision, At: time.Now().UTC(), Severity: AlertInfo}
	if v.Decision == "APPROVED" {
		alert.Detail = fmt.Sprintf("The display name %q was approved.", v.RequestedName)
	} else {
		alert.Severity = AlertWarning
		alert.Detail = fmt.Sprintf("The display name %q was not approved: %s", v.RequestedName, strings.ToLower(strings.ReplaceAll(v.RejectionReason, "_", " ")))
	}
	return WAIDFromPhone(v.DisplayPhoneNumber), alert, true
}

// LimitLabel describes a messaging limit tier.
func LimitLabel(tier string) string {
	switch tier {
	case "TIER_50":
		return "50 people a day"
	case "TIER_250":
		return "250 people a day"
	case "TIER_1K":
		return "1,000 people a day"
	case "TIER_10K":
		return "10,000 people a day"
	case "TIER_100K":
		return "100,000 people a day"
	case "TIER_UNLIMITED":
		return "unlimited"
	}
	return strings.ToLower(tier)
}
