package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateErrorMessageReplacementOption(t *testing.T) {
	require.NoError(t, validateOptionValue("ErrorMessageReplacementRules", `[{"match":"upstream","mode":"contains","replace":"client"}]`))
	require.Error(t, validateOptionValue("ErrorMessageReplacementRules", `[{"match":"upstream","mode":"unknown","replace":"client"}]`))
}
