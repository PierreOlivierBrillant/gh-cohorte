package ghapi

import (
	"math"
	"net/http"
	"strconv"
	"time"
)

// Bornes des tentatives automatiques sur erreur transitoire.
const (
	maxAttempts     = 4
	maxSleepSeconds = 60.0
)

// retryTransport retente les erreurs transitoires : 5xx, quotas atteints et
// limites secondaires, que GitHub applique aux écritures en rafale.
type retryTransport struct {
	base  http.RoundTripper
	sleep func(time.Duration)
	now   func() time.Time
}

func (t *retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	sleep := t.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	now := t.now
	if now == nil {
		now = time.Now
	}

	var lastResponse *http.Response
	var lastError error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 && request.GetBody != nil {
			body, err := request.GetBody()
			if err != nil {
				return lastResponse, lastError
			}
			request.Body = body
		}
		lastResponse, lastError = t.base.RoundTrip(request)
		if lastError != nil {
			if attempt == maxAttempts {
				return nil, lastError
			}
			sleep(backoff(attempt))
			continue
		}
		delay, retry := retryDelay(lastResponse, attempt, now())
		if !retry || attempt == maxAttempts {
			return lastResponse, nil
		}
		// La réponse est abandonnée : son corps doit être fermé pour libérer la connexion.
		lastResponse.Body.Close()
		sleep(delay)
	}
	return lastResponse, lastError
}

func backoff(attempt int) time.Duration {
	seconds := math.Min(math.Pow(2, float64(attempt)), maxSleepSeconds)
	return time.Duration(seconds * float64(time.Second))
}

// retryDelay indique s'il faut retenter, et après combien de temps.
func retryDelay(response *http.Response, attempt int, now time.Time) (time.Duration, bool) {
	status := response.StatusCode
	if status >= 500 {
		return backoff(attempt), true
	}
	if status != 403 && status != 429 {
		return 0, false
	}
	// Limite secondaire : GitHub indique explicitement combien de temps patienter.
	if value := response.Header.Get("Retry-After"); value != "" {
		if seconds, err := strconv.Atoi(value); err == nil {
			return time.Duration(math.Min(float64(seconds), maxSleepSeconds) * float64(time.Second)), true
		}
	}
	if response.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(response.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			wait := time.Unix(reset, 0).Sub(now) + time.Second
			if wait < 0 {
				wait = 0
			}
			if wait > maxSleepSeconds*time.Second {
				wait = maxSleepSeconds * time.Second
			}
			return wait, true
		}
	}
	return 0, false
}
