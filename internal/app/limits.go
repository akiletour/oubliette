package app

import (
	"cmp"
	"flag"
	"fmt"
	"strconv"
	"time"
)

const (
	defaultRetentionDays = 30
	defaultInboxCap      = 1000
	defaultMaxMessageMB  = 25
	maxRetentionDays     = 36500
	maxInboxCap          = 1_000_000
	maxMessageMB         = 100
	megabyte             = 1 << 20
	day                  = 24 * time.Hour
	defaultPurgeEvery    = time.Minute
)

type limit struct {
	value, max int
}

func (l *limit) String() string {
	return strconv.Itoa(l.value)
}

func (l *limit) Set(text string) error {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > l.max {
		return fmt.Errorf("must be a whole number from 1 to %d", l.max)
	}
	l.value = n
	return nil
}

type limitSettings struct {
	retentionDays, inboxCap, maxMessageMB limit
	unreadable                            error
}

func registerLimits(flags *flag.FlagSet, getenv func(string) string) *limitSettings {
	limits := &limitSettings{
		retentionDays: limit{defaultRetentionDays, maxRetentionDays},
		inboxCap:      limit{defaultInboxCap, maxInboxCap},
		maxMessageMB:  limit{defaultMaxMessageMB, maxMessageMB},
	}
	for _, setting := range []struct {
		value                 *limit
		name, variable, usage string
	}{
		{&limits.retentionDays, "retention-days", "OUBLIETTE_RETENTION_DAYS", "days a Message is kept before it is purged"},
		{&limits.inboxCap, "inbox-cap", "OUBLIETTE_INBOX_CAP", "Messages an Inbox holds, the oldest leaving first"},
		{&limits.maxMessageMB, "max-message-mb", "OUBLIETTE_MAX_MESSAGE_MB", fmt.Sprintf("largest Message accepted, in MB of %d bytes", megabyte)},
	} {
		if text := getenv(setting.variable); text != "" {
			if err := setting.value.Set(text); err != nil && limits.unreadable == nil {
				limits.unreadable = fmt.Errorf("%s %w", setting.variable, err)
			}
		}
		flags.Var(setting.value, setting.name, setting.usage+" ("+setting.variable+")")
	}
	return limits
}

func (l *limitSettings) apply(cfg Config) (Config, error) {
	cfg.AgeLimit = time.Duration(l.retentionDays.value) * day
	cfg.InboxCap = l.inboxCap.value
	cfg.MaxMessageBytes = int64(l.maxMessageMB.value) * megabyte
	return cfg, l.unreadable
}

func withDefaultLimits(cfg Config) Config {
	cfg.InboxCap = cmp.Or(cfg.InboxCap, defaultInboxCap)
	cfg.MaxMessageBytes = cmp.Or(cfg.MaxMessageBytes, defaultMaxMessageMB*megabyte)
	cfg.AgeLimit = cmp.Or(cfg.AgeLimit, defaultRetentionDays*day)
	cfg.PurgeEvery = cmp.Or(cfg.PurgeEvery, defaultPurgeEvery)
	return cfg
}
