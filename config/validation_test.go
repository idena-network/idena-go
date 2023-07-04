package config

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGetNextValidationTime(t *testing.T) {
	conf := &ValidationConfig{}
	validationTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	nextValidationTime := conf.GetNextValidationTime(validationTime, 449, true)
	require.Equal(t, time.Date(2020, 1, 16, 0, 0, 0, 0, time.UTC),
		nextValidationTime)

	nextValidationTime = conf.GetNextValidationTime(time.Date(2020, 1, 1, 13, 30, 0, 0, time.UTC), 449, true)
	require.Equal(t, time.Date(2020, 1, 16, 15, 0, 0, 0, time.UTC),
		nextValidationTime)

	nextValidationTime = conf.GetNextValidationTime(time.Date(2020, 1, 1, 15, 0, 0, 0, time.UTC), 449, true)
	require.Equal(t, time.Date(2020, 1, 16, 15, 0, 0, 0, time.UTC),
		nextValidationTime)

	conf.ValidationInterval = 2*time.Hour + time.Minute*3
	nextValidationTime = conf.GetNextValidationTime(validationTime, 449, true)
	require.Equal(t, time.Date(2020, 1, 1, 2, 3, 0, 0, time.UTC),
		nextValidationTime)
}
