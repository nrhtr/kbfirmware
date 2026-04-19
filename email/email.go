package email

import (
	"fmt"
	"log"
	"net/smtp"
	"strings"
	"time"

	"kbfirmware/db"
)

// Config holds SMTP configuration for sending digest emails.
type Config struct {
	Host string
	Port string
	User string
	Pass string
	From string
	To   string
}

// StartDailyDigest starts a goroutine that sends a daily digest email at 6:30pm Melbourne time.
func StartDailyDigest(cfg Config, database *db.DB) {
	go func() {
		loc, err := time.LoadLocation("Australia/Melbourne")
		if err != nil {
			log.Printf("email: failed to load Australia/Melbourne timezone: %v", err)
			return
		}

		for {
			next := nextDigestTime(loc)
			log.Printf("email: next digest scheduled at %s", next.Format(time.RFC1123))
			time.Sleep(time.Until(next))

			flags, err := database.OpenFlags()
			if err != nil {
				log.Printf("email: failed to fetch open flags: %v", err)
				// Still loop — try again tomorrow
			} else if len(flags) == 0 {
				log.Printf("email: no pending flags, skipping digest")
			} else {
				if cfg.To == "" {
					log.Printf("email: SMTP_TO is not configured, skipping digest (have %d flags)", len(flags))
				} else {
					if err := SendDigest(cfg, flags); err != nil {
						log.Printf("email: failed to send digest: %v", err)
					} else {
						log.Printf("email: sent digest with %d flag(s)", len(flags))
					}
				}
			}
		}
	}()
}

// nextDigestTime computes the next 6:30pm Melbourne time.
// If the current Melbourne time is before 6:30pm today, it uses today; otherwise tomorrow.
func nextDigestTime(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	target := time.Date(now.Year(), now.Month(), now.Day(), 18, 30, 0, 0, loc)
	if !now.Before(target) {
		target = target.Add(24 * time.Hour)
	}
	return target
}

// SendDigest composes and sends a plain-text digest email listing pending flags.
func SendDigest(cfg Config, flags []db.Flag) error {
	if cfg.To == "" {
		return fmt.Errorf("SMTP_TO is not configured")
	}

	subject := fmt.Sprintf("kbfirmware: %d flag(s) pending review", len(flags))

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("kbfirmware daily digest — %d flag(s) pending review\n", len(flags)))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	for _, fl := range flags {
		sb.WriteString(fmt.Sprintf("Flag #%d\n", fl.ID))
		sb.WriteString(fmt.Sprintf("  PCB:      %s\n", fl.PCBName))
		sb.WriteString(fmt.Sprintf("  Firmware: %s\n", fl.FirmwareName))
		sb.WriteString(fmt.Sprintf("  Reason:   %s\n", fl.Reason))
		sb.WriteString(fmt.Sprintf("  Reporter: %s\n", fl.ReporterIP))
		sb.WriteString(fmt.Sprintf("  Time:     %s\n", time.Unix(fl.CreatedAt, 0).UTC().Format(time.RFC1123)))
		sb.WriteString("\n")
	}

	body := sb.String()

	msg := "From: " + cfg.From + "\r\n" +
		"To: " + cfg.To + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"\r\n" +
		body

	addr := cfg.Host + ":" + cfg.Port
	if cfg.Port == "" {
		addr = cfg.Host + ":25"
	}

	var auth smtp.Auth
	if cfg.User != "" && cfg.Pass != "" {
		auth = smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
	}

	return smtp.SendMail(addr, auth, cfg.From, []string{cfg.To}, []byte(msg))
}
