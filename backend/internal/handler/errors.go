package handler

import (
	"errors"
	"net/http"

	"github.com/blueship581/print-color-calibration-release/backend/internal/repository"
	"github.com/blueship581/print-color-calibration-release/backend/internal/service"
	"github.com/blueship581/print-color-calibration-release/backend/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput),
		errors.Is(err, service.ErrProofNotLinked), errors.Is(err, service.ErrRunNotGateable),
		errors.Is(err, service.ErrToleranceMissing):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	case errors.Is(err, service.ErrForbidden):
		util.Fail(c, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, service.ErrLocked), errors.Is(err, service.ErrProofGateBlocked):
		util.Fail(c, http.StatusConflict, "record_locked", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
