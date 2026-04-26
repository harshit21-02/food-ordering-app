// Package mailer sends email via SMTP using only the standard library.
//
// In dev, SMTP is usually unconfigured — Mailer.Send becomes a no-op and the
// caller falls back to returning the OTP in the API response (`dev_otp`).
// In any non-dev environment, set SMTP_HOST/PORT/USER/PASS/FROM in `.env`.
//
// Tested SMTP providers (any standard SMTP server works):
//
//   Gmail (port 587, STARTTLS):
//     SMTP_HOST=smtp.gmail.com  SMTP_PORT=587
//     SMTP_USER=<your-gmail>    SMTP_PASS=<gmail app password, NOT your account password>
//     SMTP_FROM="Tealogy Cafe <your-gmail>"
//
//   Resend (free 3k/mo, port 587):
//     SMTP_HOST=smtp.resend.com SMTP_PORT=587
//     SMTP_USER=resend          SMTP_PASS=<resend api key>
//     SMTP_FROM="Tealogy Cafe <onboarding@resend.dev>"
package mailer

import (
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
)

// Config holds SMTP credentials. If Host is empty, the mailer is "disabled"
// and Send returns ErrDisabled — callers should treat that as a soft skip.
type Config struct {
	Host string
	Port int
	User string
	Pass string
	From string
}

// ErrDisabled is returned when SMTP is not configured (Host empty).
var ErrDisabled = errors.New("mailer: SMTP not configured")

type Mailer struct {
	cfg Config
}

func New(cfg Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// Enabled reports whether SMTP is configured to send.
func (m *Mailer) Enabled() bool {
	return m != nil && m.cfg.Host != ""
}

// Send delivers a plain-text email. Returns ErrDisabled if SMTP isn't configured.
//
// SMTP_FROM may be a bare address ("foo@bar.com") or a display-name form
// ("Tealogy Cafe <foo@bar.com>"). The envelope sender (MAIL FROM) must always
// be a bare address — Gmail rejects the display-name form there with
// "555 5.5.2 Syntax error". The "From:" header inside the message keeps the
// formatted version so recipients see the friendly name.
func (m *Mailer) Send(to, subject, body string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	rawFrom := m.cfg.From
	if rawFrom == "" {
		rawFrom = m.cfg.User
	}
	parsed, err := mail.ParseAddress(rawFrom)
	if err != nil {
		// Fall back to treating the raw string as a bare address.
		parsed = &mail.Address{Address: rawFrom}
	}

	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	auth := smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)

	msg := []byte(
		"From: " + parsed.String() + "\r\n" +
			"To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"MIME-Version: 1.0\r\n" +
			"Content-Type: text/plain; charset=\"utf-8\"\r\n" +
			"\r\n" +
			body + "\r\n",
	)
	return smtp.SendMail(addr, auth, parsed.Address, []string{to}, msg)
}
