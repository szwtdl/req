package client

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
)

func TestToString(t *testing.T) {
	if ToString(1) != "1" {
		t.Error("ToString(1) failed")
	}
	if ToString("hello") != "hello" {
		t.Error("ToString(hello) failed")
	}
	if ToString(3.14) != "3.14" {
		t.Error("ToString(3.14) failed")
	}
}

// ----- IsTimeoutError -----

func TestIsTimeoutError_DeadlineExceeded(t *testing.T) {
	if !IsTimeoutError(context.DeadlineExceeded) {
		t.Fatal("context.DeadlineExceeded should be timeout error")
	}
}

func TestIsTimeoutError_False(t *testing.T) {
	if IsTimeoutError(errors.New("some error")) {
		t.Fatal("generic error should not be timeout")
	}
}

func TestIsTimeoutError_Nil(t *testing.T) {
	if IsTimeoutError(nil) {
		t.Fatal("nil should not be timeout error")
	}
}

// ----- IsDNSError -----

func TestIsDNSError_True(t *testing.T) {
	dnsErr := &net.DNSError{Name: "badhost", Err: "no such host"}
	if !IsDNSError(dnsErr) {
		t.Fatal("should detect DNS error")
	}
}

func TestIsDNSError_False(t *testing.T) {
	if IsDNSError(errors.New("not dns")) {
		t.Fatal("should not be DNS error")
	}
}

// ----- IsConnectionRefused -----

func TestIsConnectionRefused_True(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED},
	}
	if !IsConnectionRefused(err) {
		t.Fatal("should detect connection refused")
	}
}

func TestIsConnectionRefused_False(t *testing.T) {
	if IsConnectionRefused(errors.New("no")) {
		t.Fatal("generic error should not be connection refused")
	}
}

// ----- IsNetworkUnreachable -----

func TestIsNetworkUnreachable_True(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Err: &os.SyscallError{Syscall: "connect", Err: syscall.ENETUNREACH},
	}
	if !IsNetworkUnreachable(err) {
		t.Fatal("should detect network unreachable")
	}
}

func TestIsNetworkUnreachable_False(t *testing.T) {
	if IsNetworkUnreachable(errors.New("no")) {
		t.Fatal("generic error should not be network unreachable")
	}
}

// ----- IsInvalidAddressError -----

func TestIsInvalidAddressError_True(t *testing.T) {
	err := &net.AddrError{Err: "invalid address", Addr: ":::bad"}
	if !IsInvalidAddressError(err) {
		t.Fatal("should detect invalid address error")
	}
}

func TestIsInvalidAddressError_False(t *testing.T) {
	if IsInvalidAddressError(errors.New("other")) {
		t.Fatal("should not be address error")
	}
}

// ----- IsRetryableError -----

func TestIsRetryableError_Nil(t *testing.T) {
	if IsRetryableError(nil) {
		t.Fatal("nil should not be retryable")
	}
}

func TestIsRetryableError_EOF(t *testing.T) {
	if !IsRetryableError(io.EOF) {
		t.Fatal("EOF should be retryable")
	}
}

func TestIsRetryableError_UnexpectedEOF(t *testing.T) {
	if !IsRetryableError(io.ErrUnexpectedEOF) {
		t.Fatal("ErrUnexpectedEOF should be retryable")
	}
}

func TestIsRetryableError_ConnectionReset(t *testing.T) {
	err := &net.OpError{
		Op:  "read",
		Err: &os.SyscallError{Syscall: "read", Err: syscall.ECONNRESET},
	}
	if !IsRetryableError(err) {
		t.Fatal("ECONNRESET should be retryable")
	}
}

func TestIsRetryableError_Keyword_BrokenPipe(t *testing.T) {
	if !IsRetryableError(errors.New("broken pipe")) {
		t.Fatal("'broken pipe' message should be retryable")
	}
}

func TestIsRetryableError_Keyword_UseClosed(t *testing.T) {
	if !IsRetryableError(errors.New("use of closed network connection")) {
		t.Fatal("'use of closed network connection' should be retryable")
	}
}

func TestIsRetryableError_NotRetryable(t *testing.T) {
	if IsRetryableError(errors.New("permission denied")) {
		t.Fatal("permission denied should not be retryable")
	}
}

