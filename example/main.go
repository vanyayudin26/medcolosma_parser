package main

import (
	"context"
	"fmt"
	"time"

	hmtpk "github.com/vanyayudin26/medcolosma_parser/v2"
	"github.com/sirupsen/logrus"
)

func main() {
	logger := logrus.New()
	c := hmtpk.NewController(nil, logger)

	fmt.Println("Запуск теста парсера ОмГМУ...")
	start := time.Now()

	teachers, err := c.GetTeacherOptions(context.Background())
	if err != nil {
		fmt.Println("Ошибка:", err)
		return
	}

	fmt.Printf("\nНайдено преподавателей: %d (время: %v)\n", len(teachers), time.Since(start))

	if len(teachers) > 0 {
		// Проверяем Худякову
		target := "Худякова Н.В."
		fmt.Printf("\n--- Проверка расписания для: %s ---\n", target)

		// ПОРЯДОК: (имя, дата, контекст)
		schedule, err := c.GetScheduleByTeacher(target, "", context.Background())
		if err != nil {
			fmt.Println("Ошибка запроса:", err)
			return
		}

		fmt.Printf("Найдено дней с парами: %d\n", len(schedule))
	}
}