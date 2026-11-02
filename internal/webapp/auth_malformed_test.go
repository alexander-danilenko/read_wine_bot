package webapp_test

import (
	"errors"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

// Launch data that is signed correctly but malformed is still rejected.
func TestVerifyMalformedFields(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	authDate := strconv.FormatInt(now.Unix(), 10)
	signed := func(fields url.Values) string { return signInitData(t, testToken, fields) }

	tests := []struct {
		name     string
		initData string
		want     error
	}{
		{
			name: "repeated key",
			initData: signed(
				url.Values{"auth_date": {authDate}, "user": {`{"id":1}`}},
			) + "&user=" + url.QueryEscape(
				`{"id":2}`,
			),
			want: webapp.ErrInitDataMalformed,
		},
		{
			name:     "auth_date not a number",
			initData: signed(url.Values{"auth_date": {"soon"}, "user": {`{"id":1}`}}),
			want:     webapp.ErrInitDataMalformed,
		},
		{
			name:     "user not JSON",
			initData: signed(url.Values{"auth_date": {authDate}, "user": {"{"}}),
			want:     webapp.ErrInitDataNoUser,
		},
		{
			name:     "user without id",
			initData: signed(url.Values{"auth_date": {authDate}, "user": {`{"first_name":"Ann"}`}}),
			want:     webapp.ErrInitDataNoUser,
		},
		{name: "no hash", initData: "auth_date=" + authDate, want: webapp.ErrInitDataMalformed},
		{name: "hash not hex", initData: "auth_date=" + authDate + "&hash=zz", want: webapp.ErrInitDataMalformed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := webapp.Verify(tc.initData, testToken, now); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}
