package app

import (
	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/platform/auth"
	"github.com/flyluman/aelora/internal/platform/config"
	"go.uber.org/zap"
)

type App struct {
	SendMessage   *application.SendMessageUseCase
	SyncMessages  *application.SyncMessagesUseCase
	AckMessage    *application.AckMessageUseCase
	CreateRoom    *application.CreateRoomUseCase
	JoinRoom      *application.JoinRoomUseCase
	ListMessages  *application.ListMessagesUseCase
	RegisterPush  *application.RegisterPushTokenUseCase
	DeletePush    *application.DeletePushTokenUseCase
	NotifyOffline *application.NotifyOfflineMembersUseCase

	Cfg  config.Config
	Log  *zap.Logger
	Auth auth.Validator
}

func New(
	send *application.SendMessageUseCase,
	sync *application.SyncMessagesUseCase,
	ack *application.AckMessageUseCase,
	createRoom *application.CreateRoomUseCase,
	joinRoom *application.JoinRoomUseCase,
	listMessages *application.ListMessagesUseCase,
	registerPush *application.RegisterPushTokenUseCase,
	deletePush *application.DeletePushTokenUseCase,
	notifyOffline *application.NotifyOfflineMembersUseCase,
	cfg config.Config,
	log *zap.Logger,
	authn auth.Validator,
) *App {
	return &App{
		SendMessage:   send,
		SyncMessages:  sync,
		AckMessage:    ack,
		CreateRoom:    createRoom,
		JoinRoom:      joinRoom,
		ListMessages:  listMessages,
		RegisterPush:  registerPush,
		DeletePush:    deletePush,
		NotifyOffline: notifyOffline,
		Cfg:           cfg,
		Log:           log,
		Auth:          authn,
	}
}
