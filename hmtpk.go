package hmtpk_parser

import (
	"context"

	"github.com/vanyayudin26/medcolosma_parser/v2/announce"
	"github.com/vanyayudin26/medcolosma_parser/v2/errors"
	"github.com/vanyayudin26/medcolosma_parser/v2/model"
	"github.com/vanyayudin26/medcolosma_parser/v2/schedule"
	"github.com/vanyayudin26/medcolosma_parser/v2/schedule/group"
	"github.com/vanyayudin26/medcolosma_parser/v2/schedule/teacher"
	"github.com/vanyayudin26/medcolosma_parser/v2/storage"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	r        *storage.Redis
	log      *logrus.Logger
	group    *group.Controller
	teacher  *teacher.Controller
	announce *announce.Announce
}

func NewController(client *redis.Client, logger *logrus.Logger) *Controller {
	return &Controller{
		r:        &storage.Redis{Redis: client},
		log:      logger,
		group:    group.NewController(client, logger),
		teacher:  teacher.NewController(client, logger),
		announce: announce.NewAnnounce(logger),
	}
}

func (c *Controller) GetScheduleByGroup(group, date string, ctx context.Context) ([]model.Schedule, error) {
	return c.getSchedule(ctx, group, date, c.group)
}

func (c *Controller) GetScheduleByTeacher(teacher, date string, ctx context.Context) ([]model.Schedule, error) {
	return c.getSchedule(ctx, teacher, date, c.teacher)
}

func (c *Controller) GetGroupOptions(ctx context.Context) ([]model.Option, error) {
	return c.group.GetOptions(ctx)
}

func (c *Controller) GetTeacherOptions(ctx context.Context) ([]model.Option, error) {
	return c.teacher.GetOptions(ctx)
}

func (c *Controller) getSchedule(ctx context.Context, name, date string, adapter schedule.Adapter) ([]model.Schedule, error) {
	if name == "0" || name == "" {
		return nil, errors.ErrorBadRequest
	}

	return adapter.GetSchedule(ctx, name, date)
}

func (c *Controller) GetAnnounces(ctx context.Context, page int) (model.Announces, error) {
	if page < 1 {
		return model.Announces{}, errors.ErrorBadRequest
	}

	return c.announce.GetAnnounces(ctx, page)
}

// === НОВЫЕ МЕТОДЫ === //

func (c *Controller) GetLastUpdateDate(ctx context.Context) (string, error) {
	return c.group.GetLastUpdateDate(ctx)
}

func (c *Controller) ClearCache(ctx context.Context) error {
	return c.group.ClearCache(ctx)
}