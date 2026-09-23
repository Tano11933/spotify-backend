package middleware

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"

	"spotify-backend/pkg/apperr"
	"spotify-backend/pkg/response"
)

var rateLimitScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return {current, redis.call("PTTL", KEYS[1])}
`)

type RateLimiter struct {
	rdb *redis.Client
}

func NewRateLimiter(rdb *redis.Client) *RateLimiter {
	return &RateLimiter{rdb: rdb}
}

type RateLimitConfig struct {
	Name string
	Max int
	Window time.Duration
	KeyFunc func(c *fiber.Ctx) string
}

func (rl *RateLimiter) Limit(cfg RateLimitConfig) fiber.Handler {
	if cfg.KeyFunc == nil {
		cfg.KeyFunc = KeyByIP
	}

	return func(c *fiber.Ctx) error {
		key := fmt.Sprintf("rate_limit:%s:%s", cfg.Name, cfg.KeyFunc(c))

		result, err := rateLimitScript.Run(
			c.UserContext(),
			rl.rdb,
			[]string{key},
			cfg.Window.Milliseconds(),
		).Slice()

		if err != nil {
			log.Printf("rate limiter unavailable for %s, allowing request: %v", key, err)
			return c.Next()
		}

		count, ttl := parseRateLimitResult(result)

		remaining := cfg.Max - int(count)
		if remaining < 0 {
			remaining = 0
		}
		c.Set("X-RateLimit-Limit", strconv.Itoa(cfg.Max))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if int(count) > cfg.Max {
			retryAfter := int(time.Duration(ttl * int64(time.Millisecond)).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Set(fiber.HeaderRetryAfter, strconv.Itoa(retryAfter))

			return response.Error(c, fiber.StatusTooManyRequests, apperr.CodeRateLimited,
				fmt.Sprintf("too many requests, try again in %d seconds", retryAfter))
		}

		return c.Next()
	}
}

func parseRateLimitResult(result []any) (count int64, ttlMillis int64) {
	if len(result) > 0 {
		if v, ok := result[0].(int64); ok {
			count = v
		}
	}
	if len(result) > 1 {
		if v, ok := result[1].(int64); ok {
			ttlMillis = v
		}
	}
	return count, ttlMillis
}

func KeyByIP(c *fiber.Ctx) string {
	return "ip:" + c.IP()
}

func KeyByJSONField(field string) func(c *fiber.Ctx) string {
	return func(c *fiber.Ctx) string {
		var payload map[string]any
		if err := json.Unmarshal(c.Body(), &payload); err != nil {
			return "ip:" + c.IP()
		}

		value, ok := payload[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return "ip:" + c.IP()
		}

		return field + ":" + strings.ToLower(strings.TrimSpace(value))
	}
}
