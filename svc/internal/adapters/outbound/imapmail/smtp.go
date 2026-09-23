package imapmail

import (
	"context"
	"errors"
	"fmt"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
)

// smtpSession dials, secures and authenticates, ready for MAIL FROM.
func (p *Provider) smtpSession(ctx context.Context, c credential) (*smtp.Client, error) {
	conn, err := p.dial(ctx, c.SMTP)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr(c.SMTP), err)
	}
	var sc *smtp.Client
	if c.SMTP.Security == SecurityStartTLS {
		if sc, err = smtp.NewClientStartTLS(conn, p.tlsConfig(c.SMTP.Host)); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("smtp starttls: %w", err)
		}
	} else {
		sc = smtp.NewClient(conn)
	}
	sc.CommandTimeout = p.commandTimeout()
	sc.SubmissionTimeout = p.commandTimeout()
	// PLAIN where offered; LOGIN is what some older Exchange servers insist on.
	var auth sasl.Client
	if ok, _ := sc.Extension("AUTH"); ok && !sc.SupportsAuth(sasl.Plain) && sc.SupportsAuth(sasl.Login) {
		auth = sasl.NewLoginClient(c.Username, c.Password)
	} else {
		auth = sasl.NewPlainClient("", c.Username, c.Password)
	}
	if err := sc.Auth(auth); err != nil {
		_ = sc.Close()
		var se *smtp.SMTPError
		if errors.As(err, &se) && se.Code == 535 {
			return nil, fmt.Errorf("%w: %s", errLoginRefused, se.Message)
		}
		return nil, fmt.Errorf("smtp auth: %w", err)
	}
	return sc, nil
}

// notSent marks a failure that happened before the server accepted anything.
func notSent(err error) error {
	if errors.Is(err, driven.ErrMailNotSent) {
		return err
	}
	return fmt.Errorf("%w: %v", driven.ErrMailNotSent, err)
}

// send submits a composed message. SMTP makes the not-sent boundary exact:
// nothing is delivered until the server answers the end of DATA, so every
// failure before that — and a refusal in that answer — is not-sent. Only a
// connection lost while waiting for the answer leaves the outcome unknown.
func (p *Provider) send(ctx context.Context, c credential, from string, to []string, raw []byte) error {
	if int64(len(raw)) > p.maxRaw() {
		return driven.TooLarge(p.maxRaw())
	}
	if len(to) == 0 {
		return notSent(fmt.Errorf("no recipients"))
	}
	sc, err := p.smtpSession(ctx, c)
	if err != nil {
		return notSent(err)
	}
	defer sc.Close()
	if limit, ok := sc.MaxMessageSize(); ok && limit > 0 && len(raw) > limit {
		return driven.TooLarge(int64(limit))
	}
	if err := sc.Mail(from, &smtp.MailOptions{Size: int64(len(raw))}); err != nil {
		return notSent(fmt.Errorf("smtp mail from: %w", err))
	}
	for _, rcpt := range to {
		if err := sc.Rcpt(rcpt, nil); err != nil {
			return notSent(fmt.Errorf("smtp rcpt %s: %w", rcpt, err))
		}
	}
	w, err := sc.Data()
	if err != nil {
		return notSent(fmt.Errorf("smtp data: %w", err))
	}
	if _, err := w.Write(raw); err != nil {
		// Without the terminating dot the server cannot deliver.
		return notSent(fmt.Errorf("smtp write: %w", err))
	}
	if err := w.Close(); err != nil {
		var se *smtp.SMTPError
		if errors.As(err, &se) {
			if se.Code == 552 {
				// 552 at end of data is the server's size limit.
				return fmt.Errorf("%w: %w: %s", driven.ErrMailNotSent, driven.ErrMailTooLarge, se.Message)
			}
			return notSent(fmt.Errorf("smtp refused message: %w", err))
		}
		return fmt.Errorf("smtp: delivery status unknown: %w", err)
	}
	_ = sc.Quit()
	return nil
}
