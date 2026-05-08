package deploy

import "time"

func disableAutoRestartBackoff(ar *AutoRestarter) {
	ar.retryDelay = func(int) time.Duration { return 0 }
}
