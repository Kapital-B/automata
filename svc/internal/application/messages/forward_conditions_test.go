package messages

import (
	"encoding/json"
	"errors"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"strings"
	"testing"
	"time"
)

// Review findings 6 and 7: an empty condition matched every message, and an
// unknown field silently matched none. Both are refused when saved.
func TestNormalizeForwardRuleRefusesWhatWouldMisbehave(t *testing.T) {
	cases := map[string]struct {
		mode, condition, want string
	}{
		"empty object":     {domainmessages.MessageForwardModeLogic, `{}`, "at least one condition"},
		"empty all":        {domainmessages.MessageForwardModeLogic, `{"all":[]}`, "at least one condition"},
		"missing":          {domainmessages.MessageForwardModeLogic, ``, "not valid"},
		"unknown field":    {domainmessages.MessageForwardModeLogic, `{"all":[{"field":"body","op":"contains","value":"x"}]}`, "unknown condition field"},
		"wrong op":         {domainmessages.MessageForwardModeLogic, `{"all":[{"field":"subject","op":"domain","value":"x"}]}`, "cannot use"},
		"bool expected":    {domainmessages.MessageForwardModeLogic, `{"all":[{"field":"has_attachments","op":"equals","value":"yes"}]}`, "true or false"},
		"empty value":      {domainmessages.MessageForwardModeLogic, `{"all":[{"field":"subject","op":"contains","value":" "}]}`, "needs a value"},
		"typo in key":      {domainmessages.MessageForwardModeLogic, `{"al":[{"field":"subject","op":"contains","value":"x"}]}`, "not valid"},
		"empty prompt":     {domainmessages.MessageForwardModeLLM, `{"prompt":"  "}`, "describe"},
		"logic json as ai": {domainmessages.MessageForwardModeLLM, `{"all":[]}`, "not valid"},
		"unknown mode":     {"regex", `{}`, "mode must be"},
	}
	for name, c := range cases {
		_, _, err := domainmessages.MessageNormalizeForwardRule(c.mode, json.RawMessage(c.condition))
		var re *domainmessages.MessageForwardRuleError
		if !errors.As(err, &re) || !strings.Contains(re.Reason, c.want) {
			t.Errorf("%s: err = %v, want a rule error mentioning %q", name, err, c.want)
		}
	}
}

func TestNormalizeForwardRuleCanonicalises(t *testing.T) {
	mode, cond, err := domainmessages.MessageNormalizeForwardRule(" Logic ", json.RawMessage(`{"all":[{"field":"From","op":"eq","value":" Billing@Vendor.Example "},{"field":"from","op":"domain","value":"@Vendor.Example"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"all":[{"field":"from","op":"equals","value":"billing@vendor.example"},{"field":"from","op":"domain","value":"vendor.example"}]}`
	if mode != domainmessages.MessageForwardModeLogic || cond != want {
		t.Fatalf("got %s %s", mode, cond)
	}
}

func TestLogicFieldsMatch(t *testing.T) {
	finance := "finance"
	msg := domainmessages.MessageRow{
		Subject: "Invoice INV-7", FromJSON: `{"name":"Acme Billing","address":"billing@eu.acme.com"}`,
		HasAttachments: true, CategorySlug: &finance, ReceivedAt: time.Now(),
	}
	cases := []struct {
		cond  string
		match bool
	}{
		{`{"all":[{"field":"from","op":"domain","value":"acme.com"}]}`, true},
		{`{"all":[{"field":"from","op":"domain","value":"cme.com"}]}`, false},
		{`{"all":[{"field":"from","op":"equals","value":"billing@eu.acme.com"}]}`, true},
		{`{"all":[{"field":"from","op":"contains","value":"acme billing"}]}`, true},
		{`{"all":[{"field":"subject","op":"contains","value":"invoice"}]}`, true},
		{`{"all":[{"field":"subject","op":"equals","value":"invoice"}]}`, false},
		{`{"all":[{"field":"has_attachments","op":"equals","value":true},{"field":"category_slug","op":"equals","value":"finance"}]}`, true},
		{`{"all":[{"field":"has_attachments","op":"equals","value":false}]}`, false},
	}
	for _, c := range cases {
		_, norm, err := domainmessages.MessageNormalizeForwardRule(domainmessages.MessageForwardModeLogic, json.RawMessage(c.cond))
		if err != nil {
			t.Fatalf("%s: %v", c.cond, err)
		}
		v, err := evaluateLogic(norm, msg, time.Now())
		if err != nil || v.match != c.match {
			t.Errorf("%s: match = %v (%v), want %v", c.cond, v.match, err, c.match)
		}
	}
}
