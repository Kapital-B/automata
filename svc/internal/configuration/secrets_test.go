package configuration

import (
	"errors"
	"testing"

	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

func TestIsUnpopulatedSecret(t *testing.T) {
	t.Parallel()

	if !isUnpopulatedSecret(&smtypes.ResourceNotFoundException{
		Message: awsString("Secrets Manager can't find the specified secret value for staging label: AWSCURRENT"),
	}) {
		t.Fatal("ResourceNotFoundException should be treated as unpopulated")
	}
	if !isUnpopulatedSecret(errEmptySecretString) {
		t.Fatal("errEmptySecretString should be treated as unpopulated")
	}
	wrapped := errors.New(`secretsmanager GetSecretValue "automata/dev/SLACK_CLIENT_SECRET": operation error Secrets Manager: GetSecretValue, ResourceNotFoundException: AWSCURRENT`)
	if !isUnpopulatedSecret(wrapped) {
		t.Fatal("AWSCURRENT error text should be treated as unpopulated")
	}
	if isUnpopulatedSecret(errors.New("access denied")) {
		t.Fatal("unrelated errors must not soft-fail")
	}
}

func awsString(v string) *string { return &v }
