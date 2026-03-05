package application

import (
	"errors"
	"net/http"
)

type AppError struct {
	Err        error
	HTTPStatus int
	Code       string
}

func (e *AppError) Error() string { return e.Err.Error() }
func (e *AppError) Unwrap() error { return e.Err }

var (
	ErrDuplicateMessage  = &AppError{Err: errors.New("duplicate client message id"), HTTPStatus: http.StatusConflict, Code: "DUPLICATE_MESSAGE"}
	ErrRateLimited       = &AppError{Err: errors.New("rate limit exceeded"), HTTPStatus: http.StatusTooManyRequests, Code: "RATE_LIMITED"}
	ErrRoomAccessDenied  = &AppError{Err: errors.New("user is not a room member"), HTTPStatus: http.StatusForbidden, Code: "ROOM_ACCESS_DENIED"}
	ErrReplyTarget       = &AppError{Err: errors.New("reply target not found in room"), HTTPStatus: http.StatusBadRequest, Code: "REPLY_TARGET_NOT_FOUND"}
	ErrInvalidPushToken  = &AppError{Err: errors.New("invalid push token payload"), HTTPStatus: http.StatusBadRequest, Code: "INVALID_PUSH_TOKEN"}
	ErrUserAlreadyExists = &AppError{Err: errors.New("user already exists in this room"), HTTPStatus: http.StatusConflict, Code: "USER_ALREADY_EXISTS"}
	ErrInvalidAck        = &AppError{Err: errors.New("invalid ack payload"), HTTPStatus: http.StatusBadRequest, Code: "INVALID_ACK"}
)
