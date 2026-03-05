package main

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpapi "github.com/flyluman/aelora/internal/adapters/inbound/http"
	postgresrepo "github.com/flyluman/aelora/internal/adapters/outbound/postgres"
	pushadapter "github.com/flyluman/aelora/internal/adapters/outbound/push"
	"github.com/flyluman/aelora/internal/adapters/outbound/roomcache"
	system "github.com/flyluman/aelora/internal/adapters/outbound/system"
	valkeyrepo "github.com/flyluman/aelora/internal/adapters/outbound/valkey"
	"github.com/flyluman/aelora/internal/app"
	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/platform/auth"
	"github.com/flyluman/aelora/internal/platform/config"
	"github.com/flyluman/aelora/internal/platform/health"
	"github.com/flyluman/aelora/internal/platform/logger"
	"github.com/flyluman/aelora/internal/platform/migrations"
	"github.com/flyluman/aelora/internal/platform/ratelimit"
	"github.com/flyluman/aelora/internal/workers"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Load()
	log := logger.New(cfg.ServiceName, cfg.AppEnv, cfg.LogLevel)
	defer func() { _ = log.Sync() }()

	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "migrate":
		if err := runMigrations(ctx, cfg); err != nil {
			log.Fatal("migration command failed", zap.Error(err))
		}
		log.Info("migrations completed")
		return
	case "serve":
		runServer(ctx, cfg, log)
	default:
		log.Fatal("unsupported mode", zap.String("mode", mode), zap.String("supported", "serve|migrate"))
	}
}

func runMigrations(ctx context.Context, cfg config.Config) error {
	return migrations.Run(ctx, migrations.Config{
		Enabled:               true,
		PostgresDSN:           cfg.PostgresDSN,
		PostgresMigrationsDir: cfg.PostgresMigrationsDir,
	})
}

func runServer(ctx context.Context, cfg config.Config, log *zap.Logger) {
	if cfg.RunMigrationsOnStart {
		log.Info("running startup migrations")
		if err := runMigrations(context.Background(), cfg); err != nil {
			log.Fatal("startup migrations failed", zap.Error(err))
		}
	}

	// --- Infrastructure ---

	valkeyClient, err := valkeyrepo.NewClient(cfg.ValkeyAddr, cfg.ValkeyUsername, cfg.ValkeyPassword, cfg.ValkeyDB)
	if err != nil {
		log.Fatal("failed to connect valkey", zap.Error(err))
	}
	defer valkeyClient.Close()

	pool, err := postgresrepo.NewPool(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal("failed to connect postgres", zap.Error(err))
	}
	defer pool.Close()

	pgRepo, err := postgresrepo.NewRoomRepositoryFromPool(pool)
	if err != nil {
		log.Fatal("failed to init postgres repo", zap.Error(err))
	}
	defer pgRepo.Close()

	// --- Repositories ---

	vkRoomRepo := valkeyrepo.NewRoomRepository(valkeyClient)
	roomRepo := roomcache.NewRepository(pgRepo, vkRoomRepo)

	roomIDSequence := valkeyrepo.NewRoomIDSequence(valkeyClient, cfg.ValkeyRoomIDSequenceKey)
	roomIDGenerator := application.NewSequentialRoomIDGenerator(roomIDSequence, cfg.RoomIDXORMask)
	idGen := system.NewIDGenerator()
	clock := system.NewClock()

	streamRepo := valkeyrepo.NewStreamRepository(
		valkeyClient,
		cfg.ValkeyEventStreamKey,
		cfg.ValkeyRoomStreamMaxLen,
		cfg.ValkeyEventStreamMaxLen,
	)
	presenceRepo := valkeyrepo.NewPresenceRepository(valkeyClient, 30*time.Second)
	ackRepo := valkeyrepo.NewAckRepository(valkeyClient)
	pushTokenRepo := valkeyrepo.NewPushTokenRepository(valkeyClient)

	pushNotifier := pushadapter.NewHTTPNotifier(
		cfg.PushAndroidEndpoint,
		cfg.PushIOSEndpoint,
		cfg.PushAuthToken,
		time.Duration(cfg.PushHTTPTimeoutMS)*time.Millisecond,
	)

	sendLimiter := ratelimit.NewFixedWindowLimiter(cfg.SendRateLimitPerSecond, time.Second, time.Duration(cfg.SendRateLimitTTLSeconds)*time.Second)

	healthService := health.NewService(
		2*time.Second,
		health.Check{
			Name: "valkey",
			Func: func(ctx context.Context) error { return valkeyClient.Ping(ctx) },
		},
		health.Check{
			Name: "postgres",
			Func: func(ctx context.Context) error { return pgRepo.Ping(ctx) },
		},
	)

	// --- Use cases ---

	sendMessage := application.NewSendMessageUseCase(pgRepo, roomRepo, idGen, clock, streamRepo, streamRepo, pgRepo, sendLimiter)
	syncMessages := application.NewSyncMessagesUseCase(roomRepo, streamRepo, ackRepo, pgRepo)
	ackMessage := application.NewAckMessageUseCase(roomRepo, ackRepo)
	createRoom := application.NewCreateRoomUseCase(roomRepo, roomIDGenerator)
	joinRoom := application.NewJoinRoomUseCase(roomRepo)
	listMessages := application.NewListMessagesUseCase(roomRepo, pgRepo)
	registerPush := application.NewRegisterPushTokenUseCase(pushTokenRepo)
	deletePush := application.NewDeletePushTokenUseCase(pushTokenRepo)
	notifyOffline := application.NewNotifyOfflineMembersUseCase(presenceRepo, pushTokenRepo, pushNotifier)

	hub := httpapi.NewWebSocketHub(roomRepo, streamRepo, notifyOffline, log)

	var authn auth.Validator
	if cfg.AuthEnabled {
		authn = auth.NewCognitoValidator(cfg.CognitoRegion, cfg.CognitoUserPoolID, cfg.CognitoAppClientID)
	}

	// --- App ---

	a := app.New(
		sendMessage, syncMessages, ackMessage, createRoom, joinRoom, listMessages,
		registerPush, deletePush, notifyOffline,
		cfg, log, authn,
	)

	// --- HTTP handler ---

	wsHandler := httpapi.NewWebSocketHandler(
		sendMessage, syncMessages, ackMessage, presenceRepo, hub, log,
		cfg.AuthEnabled, authn, cfg.WSReadLimit, cfg.WSCheckOrigin,
	)

	handler := httpapi.NewHandler(a, hub, wsHandler, healthService)

	// --- Start ---

	handler.RunRealtime(ctx)
	go workers.NewMessageOutboxRelay(pgRepo, streamRepo, streamRepo, log).Run(ctx, 2*time.Second, 200)

	e := echo.New()
	handler.RegisterRoutes(e)

	// Frontend sidecar
	if cfg.FrontendSidecarEnabled {
		if cfg.FrontendAddr == cfg.HTTPAddr {
			log.Fatal("frontend addr must differ from api addr", zap.String("http_addr", cfg.HTTPAddr), zap.String("frontend_addr", cfg.FrontendAddr))
		}

		frontendDir, err := httpapi.ResolveFrontendDir()
		if err != nil {
			log.Fatal("failed to locate frontend assets", zap.Error(err))
		}
		frontendBackendURL, err := frontendBackendURL(cfg.HTTPAddr)
		if err != nil {
			log.Fatal("failed to derive frontend backend url", zap.Error(err), zap.String("http_addr", cfg.HTTPAddr))
		}
		frontendHandler, err := httpapi.NewFrontendSidecarHandler(frontendDir, frontendBackendURL, log)
		if err != nil {
			log.Fatal("failed to configure frontend helper", zap.Error(err))
		}

		go func() {
			log.Info("frontend helper starting", zap.String("addr", cfg.FrontendAddr), zap.String("backend_url", mustFrontendBackendURL(cfg.HTTPAddr)))
			srv := &http.Server{Addr: cfg.FrontendAddr, Handler: frontendHandler, ReadHeaderTimeout: 5 * time.Second}
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatal("frontend helper failed", zap.Error(err))
			}
		}()
	}

	// Graceful shutdown
	quit, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info("server starting", zap.String("addr", cfg.HTTPAddr))
		if err := e.Start(cfg.HTTPAddr); err != nil && err != http.ErrServerClosed {
			log.Fatal("server failed", zap.Error(err))
		}
	}()

	<-quit.Done()
	log.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = e.Shutdown(shutdownCtx)
}

func frontendBackendURL(httpAddr string) (string, error) {
	host := "127.0.0.1"
	port := ""

	switch {
	case strings.HasPrefix(httpAddr, ":"):
		port = strings.TrimPrefix(httpAddr, ":")
	case strings.Contains(httpAddr, ":"):
		addr, err := netip.ParseAddrPort(httpAddr)
		if err == nil {
			port = fmt.Sprintf("%d", addr.Port())
			if parsedHost := addr.Addr().String(); parsedHost != "0.0.0.0" && parsedHost != "::" {
				host = parsedHost
			}
			break
		}

		lastColon := strings.LastIndex(httpAddr, ":")
		if lastColon <= 0 || lastColon == len(httpAddr)-1 {
			return "", fmt.Errorf("invalid http addr %q", httpAddr)
		}
		port = httpAddr[lastColon+1:]
		parsedHost := strings.Trim(httpAddr[:lastColon], "[]")
		if parsedHost != "" && parsedHost != "0.0.0.0" && parsedHost != "::" {
			host = parsedHost
		}
	default:
		return "", fmt.Errorf("http addr %q must include port", httpAddr)
	}

	if port == "" {
		return "", fmt.Errorf("http addr %q missing port", httpAddr)
	}
	return "http://" + host + ":" + port, nil
}

func mustFrontendBackendURL(httpAddr string) string {
	url, err := frontendBackendURL(httpAddr)
	if err != nil {
		return ""
	}
	return url
}
