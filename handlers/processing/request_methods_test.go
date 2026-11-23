package processing

import (
	"errors"
	"net"
	"net/http"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/imgproxy/imgproxy/v3/errctx"
	"github.com/imgproxy/imgproxy/v3/fetcher"
)

func connectionResetError() error {
	return errctx.Wrap(
		fetcher.WrapError(&net.OpError{
			Op:  "read",
			Net: "tcp",
			Err: os.NewSyscallError("read", syscall.ECONNRESET),
		}, 0),
		errctx.WithPrefix("can't download source image"),
	)
}

func notReportableError() error {
	return errctx.NewTextError(
		"status: 404",
		1,
		errctx.WithStatusCode(http.StatusNotFound),
		errctx.WithShouldReport(false),
	)
}

func TestWrapDownloadingErr(t *testing.T) {
	tt := []struct {
		name                    string
		err                     error
		reportDownloadingErrors bool
		shouldReport            bool
	}{
		{
			name:                    "ConnectionResetReportingDisabled",
			err:                     connectionResetError(),
			reportDownloadingErrors: false,
			shouldReport:            false,
		},
		{
			name:                    "ConnectionResetReportingEnabled",
			err:                     connectionResetError(),
			reportDownloadingErrors: true,
			shouldReport:            true,
		},
		{
			name:                    "PlainErrorReportingDisabled",
			err:                     errors.New("unexpected failure"),
			reportDownloadingErrors: false,
			shouldReport:            false,
		},
		{
			name:                    "NotReportableErrorReportingEnabled",
			err:                     notReportableError(),
			reportDownloadingErrors: true,
			shouldReport:            true,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			r := &request{config: &Config{ReportDownloadingErrors: tc.reportDownloadingErrors}}

			err := r.wrapDownloadingErr(tc.err)

			require.Error(t, err)
			require.Equal(t, tc.shouldReport, err.ShouldReport())
			require.Equal(t, tc.err.Error(), err.Error())
		})
	}
}

func TestWrapDownloadingErrNil(t *testing.T) {
	r := &request{config: &Config{ReportDownloadingErrors: false}}

	require.NoError(t, r.wrapDownloadingErr(nil))
}
