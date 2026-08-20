package orchestrator

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/go-github/v74/github"
	"github.com/sethvargo/go-retry"
)

func retryOperation(ctx context.Context, operation func(context.Context) error) error {
	strategy := retry.WithMaxRetries(DefaultRetryCount, retry.NewExponential(DefaultRetryDelay))
	return retry.Do(ctx, strategy, func(retryCtx context.Context) error {
		err := operation(retryCtx)
		if err == nil {
			return nil
		}
		return classifyRetryError(err)
	})
}

func classifyRetryError(err error) error {
	if isPermanentRetryError(err) {
		return err
	}
	return retry.RetryableError(err)
}

func isPermanentRetryError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var rateLimitError *github.RateLimitError
	if errors.As(err, &rateLimitError) {
		return false
	}
	var abuseRateLimitError *github.AbuseRateLimitError
	if errors.As(err, &abuseRateLimitError) {
		return false
	}
	var responseError *github.ErrorResponse
	if !errors.As(err, &responseError) || responseError.Response == nil {
		return false
	}
	statusCode := responseError.Response.StatusCode
	isClientError := statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError
	return isClientError && statusCode != http.StatusRequestTimeout && statusCode != http.StatusTooManyRequests
}
