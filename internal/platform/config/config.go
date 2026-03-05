package config

import (
	"os"
	"strconv"
)

type Config struct {
	AppEnv                        string
	HTTPAddr                      string
	LogLevel                      string
	ServiceName                   string
	FrontendSidecarEnabled        bool
	FrontendAddr                  string
	WSReadLimit                   int64
	WSCheckOrigin                 bool
	RequestLogDefaultSuccessLevel string
	RequestLogDefaultSampleEvery  int
	RequestLogHealthSuccessLevel  string
	RequestLogHealthSampleEvery   int
	RequestLogMetricsSuccessLevel string
	RequestLogMetricsSampleEvery  int
	RequestLogMessageSuccessLevel string
	RequestLogMessageSampleEvery  int

	AuthEnabled        bool
	CognitoRegion      string
	CognitoUserPoolID  string
	CognitoAppClientID string

	ValkeyAddr              string
	ValkeyUsername          string
	ValkeyPassword          string
	ValkeyDB                int
	ValkeyEventStreamKey    string
	ValkeyRoomStreamMaxLen  int64
	ValkeyEventStreamMaxLen int64
	ValkeyRoomIDSequenceKey string
	RoomIDXORMask           uint64
	PostgresDSN             string
	PostgresMigrationsDir   string
	RunMigrationsOnStart    bool
	SendRateLimitPerSecond  int
	SendRateLimitTTLSeconds int

	PushAndroidEndpoint string
	PushIOSEndpoint     string
	PushAuthToken       string
	PushHTTPTimeoutMS   int
}

func Load() Config {
	return Config{
		AppEnv:                        getenv("AELORA_APP_ENV", "dev"),
		HTTPAddr:                      getenv("AELORA_HTTP_ADDR", ":8080"),
		LogLevel:                      getenv("AELORA_LOG_LEVEL", "info"),
		ServiceName:                   getenv("AELORA_SERVICE_NAME", "aelora"),
		FrontendSidecarEnabled:        getenvBool("AELORA_FRONTEND_SIDECAR_ENABLED", false),
		FrontendAddr:                  getenv("AELORA_FRONTEND_ADDR", ":8081"),
		WSReadLimit:                   getenvInt64("AELORA_WS_READ_LIMIT", 4096),
		WSCheckOrigin:                 getenvBool("AELORA_WS_CHECK_ORIGIN", false),
		RequestLogDefaultSuccessLevel: getenv("AELORA_HTTP_REQUEST_LOG_SUCCESS_LEVEL", "info"),
		RequestLogDefaultSampleEvery:  getenvInt("AELORA_HTTP_REQUEST_LOG_SUCCESS_SAMPLE_EVERY", 1),
		RequestLogHealthSuccessLevel:  getenv("AELORA_HTTP_REQUEST_LOG_HEALTH_LEVEL", "debug"),
		RequestLogHealthSampleEvery:   getenvInt("AELORA_HTTP_REQUEST_LOG_HEALTH_SAMPLE_EVERY", 25),
		RequestLogMetricsSuccessLevel: getenv("AELORA_HTTP_REQUEST_LOG_METRICS_LEVEL", "debug"),
		RequestLogMetricsSampleEvery:  getenvInt("AELORA_HTTP_REQUEST_LOG_METRICS_SAMPLE_EVERY", 50),
		RequestLogMessageSuccessLevel: getenv("AELORA_HTTP_REQUEST_LOG_MESSAGE_LEVEL", "info"),
		RequestLogMessageSampleEvery:  getenvInt("AELORA_HTTP_REQUEST_LOG_MESSAGE_SAMPLE_EVERY", 1),

		AuthEnabled:        getenvBool("AELORA_AUTH_ENABLED", true),
		CognitoRegion:      getenv("AELORA_COGNITO_REGION", ""),
		CognitoUserPoolID:  getenv("AELORA_COGNITO_USER_POOL_ID", ""),
		CognitoAppClientID: getenv("AELORA_COGNITO_APP_CLIENT_ID", ""),

		ValkeyAddr:              getenv("AELORA_VALKEY_ADDR", "127.0.0.1:6379"),
		ValkeyUsername:          getenv("AELORA_VALKEY_USERNAME", ""),
		ValkeyPassword:          getenv("AELORA_VALKEY_PASSWORD", ""),
		ValkeyDB:                getenvInt("AELORA_VALKEY_DB", 0),
		ValkeyEventStreamKey:    getenv("AELORA_VALKEY_EVENT_STREAM_KEY", "stream:events:message_created"),
		ValkeyRoomStreamMaxLen:  getenvInt64("AELORA_VALKEY_ROOM_STREAM_MAXLEN", 20000),
		ValkeyEventStreamMaxLen: getenvInt64("AELORA_VALKEY_EVENT_STREAM_MAXLEN", 50000),
		ValkeyRoomIDSequenceKey: getenv("AELORA_VALKEY_ROOM_ID_SEQUENCE_KEY", "seq:room_id"),
		RoomIDXORMask:           getenvUint64("AELORA_ROOM_ID_XOR_MASK", 0x7a4bd9f1c3e5),
		PostgresDSN:             getenv("AELORA_POSTGRES_DSN", ""),
		PostgresMigrationsDir:   getenv("AELORA_POSTGRES_MIGRATIONS_DIR", "migrations/postgres"),
		RunMigrationsOnStart:    getenvBool("AELORA_RUN_MIGRATIONS_ON_START", false),
		SendRateLimitPerSecond:  getenvInt("AELORA_SEND_RATE_LIMIT_PER_SECOND", 30),
		SendRateLimitTTLSeconds: getenvInt("AELORA_SEND_RATE_LIMIT_TTL_SECONDS", 300),
		PushAndroidEndpoint:     getenv("AELORA_PUSH_ANDROID_ENDPOINT", ""),
		PushIOSEndpoint:         getenv("AELORA_PUSH_IOS_ENDPOINT", ""),
		PushAuthToken:           getenv("AELORA_PUSH_AUTH_TOKEN", ""),
		PushHTTPTimeoutMS:       getenvInt("AELORA_PUSH_HTTP_TIMEOUT_MS", 2000),
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt64(key string, fallback int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getenvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getenvUint64(key string, fallback uint64) uint64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func getenvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
