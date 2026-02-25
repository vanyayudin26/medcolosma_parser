package group

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/vanyayudin26/college_osma_parser/v2/model"
	"github.com/vanyayudin26/college_osma_parser/v2/storage"
	"github.com/vanyayudin26/college_osma_parser/v2/utils"
	"github.com/go-redis/redis/v8"
	"github.com/sirupsen/logrus"
)

type Controller struct {
	r   *storage.Redis
	log *logrus.Logger
}

func NewController(client *redis.Client, logger *logrus.Logger) *Controller {
	return &Controller{r: &storage.Redis{Redis: client}, log: logger}
}

const (
	baseURL   = "https://omsk-osma.ru"
	href      = "https://omsk-osma.ru/shedule_kolledzh"
	groupsKey = "groups"
)

func formatTime(s string) string {
	s = strings.ReplaceAll(s, "-", " ")
	parts := strings.Fields(s)
	if len(parts) >= 2 {
		return parts[0] + "\n" + parts[1]
	}
	return s
}

func formatName(s string) string {
	s = strings.ReplaceAll(s, ".", ". ")
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

func getCleanText(s *goquery.Selection) string {
	html, _ := s.Html()
	replacer := strings.NewReplacer("</div>", " </div>", "<br/>", " ", "<br>", " ", "</p>", " </p>")
	cleanHTML := replacer.Replace(html)

	tmpDoc, _ := goquery.NewDocumentFromReader(strings.NewReader(cleanHTML))
	text := tmpDoc.Text()

	text = strings.ReplaceAll(text, "\u00a0", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(text), " "))
}

func (c *Controller) GetSchedule(ctx context.Context, value, date string) ([]model.Schedule, error) {
	if utils.RedisIsNil(c.r) {
		if redisData, err := c.r.Get("group_schedule:" + value); err == nil && redisData != "" {
			var schedule []model.Schedule
			if json.Unmarshal([]byte(redisData), &schedule) == nil {
				return schedule, nil
			}
		}
	}

	requestURL := value
	if !strings.HasPrefix(value, "http") {
		if strings.HasPrefix(value, "/shedule_kolledzh/") {
			requestURL = baseURL + value
		} else {
			requestURL = baseURL + "/shedule_kolledzh/" + value
		}
	}

	parsedURL, err := url.Parse(requestURL)
	if err == nil {
		requestURL = parsedURL.String()
	}

	req, err := http.NewRequestWithContext(ctx, "GET", requestURL, nil)
	if err != nil {
		return nil, err
	}

	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	scheduleTable := doc.Find("table.rasp_table")
	if scheduleTable.Length() == 0 {
		doc.Find("table").Each(func(i int, s *goquery.Selection) {
			if strings.Contains(s.Text(), "Дисциплины") {
				scheduleTable = s
			}
		})
	}

	scheduleMap := make(map[string]*model.Schedule)
	var scheduleOrder []string
	var currentDay string

	scheduleTable.Find("tr").Each(func(i int, s *goquery.Selection) {
		if i == 0 || strings.Contains(s.Text(), "Дисциплины") {
			return
		}

		cells := s.Find("td")
		var timeStr, name, teacher, room string

		if cells.Length() == 3 {
			currentDay = getCleanText(cells.Eq(0))
			timeStr = formatTime(getCleanText(cells.Eq(1)))

			infoDivs := cells.Eq(2).Find("div.cell > div")
			if infoDivs.Length() >= 3 {
				name = getCleanText(infoDivs.Eq(0))
				teacher = formatName(getCleanText(infoDivs.Eq(1)))
				room = getCleanText(infoDivs.Eq(2))
			}
		} else if cells.Length() == 2 {
			timeStr = formatTime(getCleanText(cells.Eq(0)))

			infoDivs := cells.Eq(1).Find("div.cell > div")
			if infoDivs.Length() >= 3 {
				name = getCleanText(infoDivs.Eq(0))
				teacher = formatName(getCleanText(infoDivs.Eq(1)))
				room = getCleanText(infoDivs.Eq(2))
			}
		}

		if currentDay == "" || timeStr == "" {
			return
		}

		if _, exists := scheduleMap[currentDay]; !exists {
			scheduleMap[currentDay] = &model.Schedule{
				Date:    currentDay,
				Href:    requestURL,
				Lessons: []model.Lesson{},
			}
			scheduleOrder = append(scheduleOrder, currentDay)
		}

		lessonNum := len(scheduleMap[currentDay].Lessons) + 1
		lesson := model.Lesson{
			Num:     strconv.Itoa(lessonNum),
			Time:    timeStr,
			Name:    name,
			Teacher: teacher,
			Room:    room,
		}

		scheduleMap[currentDay].Lessons = append(scheduleMap[currentDay].Lessons, lesson)
	})

	var weeklySchedule []model.Schedule
	for _, day := range scheduleOrder {
		weeklySchedule = append(weeklySchedule, *scheduleMap[day])
	}

	if utils.RedisIsNil(c.r) && len(weeklySchedule) > 0 {
		if marshal, err := json.Marshal(weeklySchedule); err == nil {
			// Увеличили кэш до недели, так как теперь есть авто-очистка
			c.r.Set("group_schedule:"+value, string(marshal), 60*24*7)
		}
	}

	return weeklySchedule, nil
}

func (c *Controller) GetOptions(ctx context.Context) (options []model.Option, err error) {
	if utils.RedisIsNil(c.r) {
		var data string
		if data, err = c.r.Get(groupsKey); err == nil && data != "" {
			if json.Unmarshal([]byte(data), &options) == nil && len(options) != 0 {
				return
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return
	}
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	doc, _ := goquery.NewDocumentFromReader(resp.Body)
	doc.Find("table a").Each(func(i int, s *goquery.Selection) {
		valueLink, exists := s.Attr("href")
		if exists && strings.Contains(valueLink, "/shedule_kolledzh/") {
			name := strings.TrimSpace(s.Text())
			options = append(options, model.Option{Label: name, Value: valueLink})
		}
	})
	if utils.RedisIsNil(c.r) && len(options) != 0 {
		if marshal, err := json.Marshal(options); err == nil {
			// Увеличили кэш до недели
			c.r.Set(groupsKey, string(marshal), 60*24*7)
		}
	}
	return
}

// Получает дату формирования расписания с сайта
func (c *Controller) GetLastUpdateDate(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", href, nil)
	if err != nil {
		return "", err
	}
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`Расписание сформировано:\s*(\d{2}\.\d{2}\.\d{4}\s\d{2}:\d{2})`)
	match := re.FindStringSubmatch(doc.Text())

	if len(match) > 1 {
		return match[1], nil
	}

	return "", fmt.Errorf("дата обновления расписания не найдена")
}

// Очищает Redis-кэш
func (c *Controller) ClearCache(ctx context.Context) error {
	if !utils.RedisIsNil(c.r) {
		return c.r.Redis.FlushDB(ctx).Err()
	}
	return nil
}