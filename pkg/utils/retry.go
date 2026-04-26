package utils

import (
	"log"
	"math"
	"time"
)

func WithRetry[T any](fn func() (T, error), maxAttempts int, baseDelay time.Duration) (T, error) {
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		result, err := fn()
		if err == nil {
			if attempt > 0 {
				log.Printf("[WithRetry] Succeeded on attempt %d/%d", attempt+1, maxAttempts)
			}
			return result, nil
		}

		lastErr = err
		isLastAttempt := attempt == maxAttempts-1

		if isLastAttempt {
			log.Printf("[WithRetry] Failed after %d attempts. Final error: %v", maxAttempts, lastErr)
			var zero T
			return zero, lastErr
		}

		delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
		log.Printf("[WithRetry] Attempt %d/%d failed: %v. Retrying in %v...", attempt+1, maxAttempts, lastErr, delay)
		time.Sleep(delay)
	}

	var zero T
	return zero, lastErr
}
