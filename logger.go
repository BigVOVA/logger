package logger

import (
	"fmt"
	"github.com/google/uuid"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Config defines the config for logger middleware
type Config struct {
	Logger *zerolog.Logger
	// UTC a boolean stating whether to use UTC time zone or local.
	UTC            bool
	SkipPath       []string
	SkipPathRegexp *regexp.Regexp
	CheckPath []string
	AppLayer       string
}

// SetLogger initializes the logging middleware.
func SetLogger(config ...Config) gin.HandlerFunc {
	var newConfig Config
	if len(config) > 0 {
		newConfig = config[0]
	}

	var check map[string]struct{}
	if length := len(newConfig.CheckPath); length > 0 {
		check = make(map[string]struct{}, length)
		for _, path := range newConfig.CheckPath {
			check[path] = struct{}{}
		}
	}

	var skip map[string]struct{}
	if length := len(newConfig.SkipPath); length > 0 {
		skip = make(map[string]struct{}, length)
		for _, path := range newConfig.SkipPath {
			skip[path] = struct{}{}
		}
	}

	var sublog zerolog.Logger
	if newConfig.Logger == nil {
		sublog = log.Logger
	} else {
		sublog = *newConfig.Logger
	}

	appLayer := "gin"

	newAppLayer := newConfig.AppLayer

	if newAppLayer != "" {
		appLayer = newAppLayer
	}

	return func(c *gin.Context) {
		start := time.Now()
		reqUuid := uuid.NewString()
		c.Set("uuid", reqUuid)
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		noCheck := true

		if _, ok := check[path]; ok {
			noCheck = false
		}

		reqInitLoggerEvent := sublog.With().
			Str("layer", appLayer).
			Str("uuid", reqUuid).
			Str("method", c.Request.Method).
			Str("path", path).
			Str("ip", c.ClientIP())

		userAgent := c.Request.UserAgent()
		if userAgent != "" {
			reqInitLoggerEvent = reqInitLoggerEvent.Str("user_agent", userAgent)
		}

		reqInitLogger := reqInitLoggerEvent.Logger()

		if noCheck {
			reqInitLogger.Debug().Msg("request detected")
		} else {
			reqInitLogger.Debug().Msg("check detected")
		}

		c.Next()
		track := true

		if _, ok := skip[path]; ok {
			track = false
		}

		if track &&
			newConfig.SkipPathRegexp != nil &&
			newConfig.SkipPathRegexp.MatchString(path) {
			track = false
		}

		if track && noCheck {
			end := time.Now()
			latency := end.Sub(start)
			if newConfig.UTC {
				end = end.UTC()
			}

			msg := "request summary"

			errors := c.Errors

			if errors != nil && len(errors) > 0 {
				errMsgs := make([]string, 0)
				for i, err := range errors {
					errMsgs = append(errMsgs, fmt.Sprintf("error #%d: %s", i+1, err.Error()))
				}
				msg = strings.Join(errMsgs, ", ")
			}

			dumpLogger := sublog.With().
				Str("layer", appLayer).
				Str("uuid", reqUuid).
				Int("status", c.Writer.Status()).
				Str("method", c.Request.Method).
				Str("path", path).
				Str("ip", c.ClientIP()).
				Dur("latency", latency).
				Str("user_agent", userAgent).
				Logger()

			switch {
			case c.Writer.Status() >= http.StatusBadRequest && c.Writer.Status() < http.StatusInternalServerError:
				{
					dumpLogger.Warn().
						Msg(msg)
				}
			case c.Writer.Status() >= http.StatusInternalServerError:
				{
					dumpLogger.Error().
						Msg(msg)
				}
			default:
				dumpLogger.Info().
					Msg(msg)
			}
		}
	}
}
