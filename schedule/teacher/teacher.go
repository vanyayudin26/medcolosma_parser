package teacher

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/vanyayudin26/college_osma_parser/v2/model"
	"github.com/vanyayudin26/college_osma_parser/v2/schedule/group"
	"github.com/vanyayudin26/college_osma_parser/v2/storage"
	"github.com/vanyayudin26/college_osma_parser/v2/utils"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	r     *storage.Redis
	log   *logrus.Logger
	group *group.Controller
}

func NewController(client *redis.Client, logger *logrus.Logger) *Controller {
	return &Controller{
		r:     &storage.Redis{Redis: client},
		log:   logger,
		group: group.NewController(client, logger),
	}
}

func (c *Controller) GetSchedule(ctx context.Context, value, date string) ([]model.Schedule, error) {
	if utils.RedisIsNil(c.r) {
		if redisData, err := c.r.Get("teacher_schedule:" + value); err == nil && redisData != "" {
			var schedule []model.Schedule
			if json.Unmarshal([]byte(redisData), &schedule) == nil {
				return schedule, nil
			}
		}
	}

	groups, err := c.group.GetOptions(ctx)
	if err != nil {
		return nil, err
	}

	scheduleMap := make(map[string]*model.Schedule)
	var scheduleOrder []string

	for _, grp := range groups {
		groupSchedule, err := c.group.GetSchedule(ctx, grp.Value, date)
		if err != nil {
			continue
		}

		for _, day := range groupSchedule {
			for _, lesson := range day.Lessons {
				if strings.Contains(lesson.Teacher, value) {
					if _, exists := scheduleMap[day.Date]; !exists {
						scheduleMap[day.Date] = &model.Schedule{
							Date:    day.Date,
							Href:    "https://omsk-osma.ru/shedule_kolledzh",
							Lessons: []model.Lesson{},
						}
						scheduleOrder = append(scheduleOrder, day.Date)
					}
					teacherLesson := lesson
					teacherLesson.Group = grp.Label
					teacherLesson.Teacher = ""

					scheduleMap[day.Date].Lessons = append(scheduleMap[day.Date].Lessons, teacherLesson)
				}
			}
		}
	}

	var weeklySchedule []model.Schedule
	for _, day := range scheduleOrder {
		sched := *scheduleMap[day]

		// ИСПРАВЛЕНИЕ БАГИ: Сортируем пары по времени (08:00 будет раньше 12:00)
		sort.Slice(sched.Lessons, func(i, j int) bool {
			return sched.Lessons[i].Time < sched.Lessons[j].Time
		})

		weeklySchedule = append(weeklySchedule, sched)
	}

	if utils.RedisIsNil(c.r) && len(weeklySchedule) > 0 {
		if marshal, err := json.Marshal(weeklySchedule); err == nil {
			// ИЗМЕНЕНО: Кэшируем расписание преподавателя на неделю (60 минут * 24 часа * 7 дней)
			c.r.Set("teacher_schedule:"+value, string(marshal), 60*24*7)
		}
	}

	return weeklySchedule, nil
}

const teachersKey = "teachers"

func (c *Controller) GetOptions(ctx context.Context) (options []model.Option, err error) {
	if utils.RedisIsNil(c.r) {
		var data string
		if data, err = c.r.Get(teachersKey); err == nil && data != "" {
			if json.Unmarshal([]byte(data), &options) == nil && len(options) != 0 {
				return
			}
		}
	}

	groups, err := c.group.GetOptions(ctx)
	if err != nil {
		return nil, err
	}

	teachersMap := make(map[string]bool)
	re := regexp.MustCompile(`[А-Я][а-яё]+\s+[А-Я]\.\s*[А-Я]\.`)

	for _, grp := range groups {
		groupSchedule, err := c.group.GetSchedule(ctx, grp.Value, "")
		if err != nil {
			continue
		}

		for _, day := range groupSchedule {
			for _, lesson := range day.Lessons {
				if lesson.Teacher != "" {
					matches := re.FindAllString(lesson.Teacher, -1)
					for _, m := range matches {
						name := strings.TrimSpace(m)
						// Красивые пробелы: "К.А." -> "К. А."
						name = strings.ReplaceAll(name, ".", ". ")
						name = strings.Join(strings.Fields(name), " ")

						if name != "" {
							teachersMap[name] = true
						}
					}
				}
			}
		}
	}

	for teacherName := range teachersMap {
		options = append(options, model.Option{Label: teacherName, Value: teacherName})
	}

	sort.Slice(options, func(i, j int) bool {
		return options[i].Label < options[j].Label
	})

	if utils.RedisIsNil(c.r) && len(options) != 0 {
		if marshal, err := json.Marshal(options); err == nil {
			// ИЗМЕНЕНО: Кэшируем список преподавателей на неделю (60 минут * 24 часа * 7 дней)
			c.r.Set(teachersKey, string(marshal), 60*24*7)
		}
	}

	return
}