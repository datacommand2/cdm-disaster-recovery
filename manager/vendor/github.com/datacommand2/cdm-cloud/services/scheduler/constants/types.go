package types

const (
	// ScheduleTypeSpecified 특정일시 스케쥴
	ScheduleTypeSpecified = "schedule.type.specified"

	// ScheduleTypeMinutely 분단위 스케줄
	ScheduleTypeMinutely = "schedule.type.minutely"

	// ScheduleTypeHourly 시간단위 스케줄
	ScheduleTypeHourly = "schedule.type.hourly"

	// ScheduleTypeDaily 일단위 스케줄
	ScheduleTypeDaily = "schedule.type.daily"

	// ScheduleTypeWeekly 주단위 스케줄
	ScheduleTypeWeekly = "schedule.type.weekly"

	// ScheduleTypeDayOfMonthly 월단위(특정 일) 스케줄
	ScheduleTypeDayOfMonthly = "schedule.type.day-of-monthly"

	// ScheduleTypeWeekOfMonthly 월단위(n번째 요일) 스케줄
	ScheduleTypeWeekOfMonthly = "schedule.type.week-of-monthly"
)
