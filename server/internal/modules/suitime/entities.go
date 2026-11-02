package suitime

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type change struct {
	Entity    string          `json:"entity"`
	ID        string          `json:"id"`
	UpdatedAt int64           `json:"updatedAt"`
	DeletedAt *int64          `json:"deletedAt"`
	DeviceID  string          `json:"deviceId"`
	Data      json.RawMessage `json:"data"`
}

type syncChange struct {
	Entity    string         `json:"entity"`
	ID        string         `json:"id"`
	UpdatedAt int64          `json:"updatedAt"`
	DeletedAt *int64         `json:"deletedAt"`
	DeviceID  string         `json:"deviceId"`
	Data      map[string]any `json:"data"`
}

var entityFields = map[string]map[string]bool{
	"category":           {"name": true, "color": true, "icon": true, "sortOrder": true},
	"task":               {"title": true, "categoryId": true, "plannedDate": true, "plannedTime": true, "plannedEndTime": true, "scheduleKind": true, "priority": true, "repeatRule": true, "occurrenceOverrides": true, "reminderOffsets": true, "sortOrder": true, "parentTaskId": true, "status": true, "failureReason": true, "notes": true, "completedAt": true},
	"anniversary":        {"kind": true, "title": true, "date": true, "notes": true, "pinned": true, "theme": true, "photos": true, "coverIndex": true},
	"anniversary_record": {"anniversaryId": true, "date": true, "notes": true, "confirmedAt": true, "title": true, "kind": true, "originalDate": true},
	"setting":            {"value": true},
}

var idPattern = regexp.MustCompile(`^[\x21-\x7e]{1,200}$`)
var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var timePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func normalizeChange(input change, fallbackDevice string, now int64, photoLimit int64) (syncChange, error) {
	// Desktop legacy preferences use a stable zero version. Promote only these
	// to the earliest positive version understood by the mobile client.
	legacySetting := input.Entity == "setting" && input.UpdatedAt == 0 && input.DeletedAt == nil
	if legacySetting {
		input.UpdatedAt = 1
	}
	if _, ok := entityFields[input.Entity]; !ok || !idPattern.MatchString(input.ID) || input.UpdatedAt <= 0 || input.UpdatedAt > 9007199254740991 {
		return syncChange{}, fmt.Errorf("实体标识或版本无效")
	}
	if input.DeletedAt != nil && (*input.DeletedAt <= 0 || *input.DeletedAt > input.UpdatedAt) {
		return syncChange{}, fmt.Errorf("删除版本无效")
	}
	if input.DeviceID == "" {
		input.DeviceID = fallbackDevice
	}
	if !validDevice(input.DeviceID) {
		return syncChange{}, fmt.Errorf("设备标识无效")
	}
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(input.Data))
	decoder.UseNumber()
	if len(input.Data) == 0 || decoder.Decode(&raw) != nil || raw == nil {
		return syncChange{}, fmt.Errorf("变更数据格式无效")
	}
	fields := entityFields[input.Entity]
	data := make(map[string]any, len(fields)+1)
	for key, value := range raw {
		if fields[key] || key == "createdAt" {
			parsed, err := parseJSONValue(value)
			if err != nil {
				return syncChange{}, fmt.Errorf("字段 %s 格式无效", key)
			}
			data[key] = parsed
		}
	}
	if legacySetting {
		if value, ok := integer(data["createdAt"]); ok && value == 0 {
			data["createdAt"] = json.Number("1")
		}
	}
	if input.Entity == "anniversary_record" && input.ID != inputRecordID(data) {
		return syncChange{}, fmt.Errorf("纪念记录标识与数据不一致")
	}
	if input.Entity == "task" && data["parentTaskId"] == input.ID {
		return syncChange{}, fmt.Errorf("事项不能引用自身")
	}
	if input.Entity == "setting" {
		if err := validateSetting(input.ID, data["value"], input.DeletedAt != nil); err != nil {
			return syncChange{}, err
		}
	}
	if err := validateFields(input.Entity, data, input.DeletedAt != nil, input.UpdatedAt, photoLimit); err != nil {
		return syncChange{}, err
	}
	return syncChange{Entity: input.Entity, ID: input.ID, UpdatedAt: input.UpdatedAt, DeletedAt: input.DeletedAt, DeviceID: input.DeviceID, Data: data}, nil
}

func parseJSONValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func validateFields(entity string, data map[string]any, deleted bool, updatedAt, photoLimit int64) error {
	required := map[string][]string{
		"category":           {"name", "color"},
		"task":               {"title", "status"},
		"anniversary":        {"kind", "title", "date"},
		"anniversary_record": {"anniversaryId", "date", "title", "kind", "originalDate"},
		"setting":            {"value"},
	}[entity]
	if deleted && entity == "setting" {
		required = nil
	}
	for _, key := range required {
		if value, ok := data[key]; !ok || value == nil {
			return fmt.Errorf("字段 %s 不能为空", key)
		}
	}
	defaults := map[string]map[string]any{
		"category":           {"icon": "tags", "sortOrder": json.Number("0")},
		"task":               {"scheduleKind": "all_day", "priority": "not_urgent_not_important", "repeatRule": `{"kind":"none"}`, "occurrenceOverrides": "{}", "reminderOffsets": []any{}, "sortOrder": json.Number("0"), "notes": ""},
		"anniversary":        {"notes": "", "pinned": false, "theme": "sky", "photos": []any{}, "coverIndex": json.Number("0")},
		"anniversary_record": {"notes": ""},
	}
	for key, value := range defaults[entity] {
		if data[key] == nil {
			data[key] = value
		}
	}
	createdAt := updatedAt
	if value := data["createdAt"]; value != nil {
		parsed, ok := integer(value)
		if !ok || parsed <= 0 || parsed > updatedAt {
			return fmt.Errorf("createdAt 无效")
		}
		createdAt = parsed
	}
	data["createdAt"] = json.Number(fmt.Sprint(createdAt))
	for _, key := range []string{"sortOrder", "coverIndex", "completedAt", "confirmedAt"} {
		if value := data[key]; value != nil {
			n, ok := integer(value)
			if !ok || n < 0 || n > 9007199254740991 {
				return fmt.Errorf("字段 %s 必须为非负安全整数", key)
			}
		}
	}
	for _, key := range []string{"categoryId", "parentTaskId", "anniversaryId"} {
		if value := data[key]; value != nil {
			id, ok := value.(string)
			if !ok || !idPattern.MatchString(id) {
				return fmt.Errorf("字段 %s 标识无效", key)
			}
		}
	}
	if entity == "task" {
		for key, options := range map[string][]string{
			"scheduleKind": {"all_day", "point", "range"},
			"status":       {"todo", "done", "failed"},
			"priority":     {"urgent_important", "important_not_urgent", "urgent_not_important", "not_urgent_not_important"},
		} {
			if !oneOf(data[key], options...) {
				return fmt.Errorf("字段 %s 枚举无效", key)
			}
		}
	}
	if entity == "anniversary" || entity == "anniversary_record" {
		if !oneOf(data["kind"], "countdown", "anniversary", "birthday", "holiday") {
			return fmt.Errorf("纪念日类型无效")
		}
	}

	switch entity {
	case "category":
		return checkText(data, "name", 40, true, "color", 32, true, "icon", 32, true)
	case "task":
		if err := checkText(data, "title", 500, true, "scheduleKind", 16, true, "priority", 32, false, "status", 16, true, "notes", 20_000, false, "failureReason", 2_000, false, "repeatRule", 4_000, true, "occurrenceOverrides", 1_000_000, true); err != nil {
			return err
		}
		for _, key := range []string{"plannedDate"} {
			if value := data[key]; value != nil && (!isString(value) || !validDate(value.(string))) {
				return fmt.Errorf("字段 %s 日期格式无效", key)
			}
		}
		for _, key := range []string{"plannedTime", "plannedEndTime"} {
			if value := data[key]; value != nil && (!isString(value) || !timePattern.MatchString(value.(string))) {
				return fmt.Errorf("字段 %s 时间格式无效", key)
			}
		}
		if a, ok := data["plannedTime"].(string); ok {
			if b, valid := data["plannedEndTime"].(string); valid && b <= a {
				return fmt.Errorf("结束时间必须晚于开始时间")
			}
		}
		if value := data["repeatRule"]; value != nil && !validJSONString(value) {
			return fmt.Errorf("repeatRule 不是合法 JSON")
		}
		if value := data["occurrenceOverrides"]; value != nil && !validJSONString(value) {
			return fmt.Errorf("occurrenceOverrides 不是合法 JSON")
		}
		if err := validateTaskJSON(data); err != nil {
			return err
		}
		if data["scheduleKind"] == "point" && (data["plannedDate"] == nil || data["plannedTime"] == nil) {
			return fmt.Errorf("时间点事项缺少日期或时间")
		}
		if data["scheduleKind"] == "range" && data["plannedDate"] == nil {
			return fmt.Errorf("时间段事项缺少日期")
		}
		if data["scheduleKind"] == "range" && data["plannedEndTime"] == nil {
			return fmt.Errorf("时间段缺少结束时间")
		}
		if value := data["reminderOffsets"]; value != nil {
			items, ok := value.([]any)
			if !ok || len(items) > 3 {
				return fmt.Errorf("提醒偏移无效")
			}
			for _, item := range items {
				if n, ok := integer(item); !ok || (n != 0 && n != 5 && n != 15 && n != 30 && n != 60 && n != 120) {
					return fmt.Errorf("提醒偏移无效")
				}
			}
		}
	case "anniversary":
		if err := checkText(data, "kind", 16, true, "title", 200, true, "notes", 20_000, false, "theme", 32, true); err != nil {
			return err
		}
		if value, ok := data["date"].(string); !ok || !validDate(value) {
			return fmt.Errorf("纪念日日期格式无效")
		}
		photos, err := photoStrings(data["photos"], photoLimit)
		if err != nil {
			return err
		}
		data["photos"] = photos
		cover, ok := integer(data["coverIndex"])
		if !ok || cover < 0 || (len(photos) == 0 && cover != 0) || (len(photos) > 0 && cover >= int64(len(photos))) {
			return fmt.Errorf("封面索引无效")
		}
		if !oneOf(data["theme"], "sky", "warm", "night") {
			return fmt.Errorf("纪念日主题无效")
		}
		if _, ok := data["pinned"].(bool); !ok {
			return fmt.Errorf("pinned 必须为布尔值")
		}
	case "anniversary_record":
		if err := checkText(data, "anniversaryId", 200, true, "title", 200, true, "kind", 16, true, "notes", 20_000, false); err != nil {
			return err
		}
		date, dateOK := data["date"].(string)
		original, originalOK := data["originalDate"].(string)
		if !dateOK || !validDate(date) || !originalOK || !validDate(original) || inputRecordID(data) == "" {
			return fmt.Errorf("纪念记录日期或标识无效")
		}
	case "setting":
		if !deleted && (data["value"] == nil) {
			return fmt.Errorf("设置值不能为空")
		}
	}
	return nil
}

func validateTaskJSON(data map[string]any) error {
	ruleValue, err := parseJSONValue([]byte(data["repeatRule"].(string)))
	if err != nil {
		return fmt.Errorf("重复规则无效")
	}
	rule, ok := ruleValue.(map[string]any)
	if !ok || !oneOf(rule["kind"], "none", "daily", "weekly", "monthly", "yearly", "workdays", "every_days", "memory", "weekly_slots", "monthly_slots", "custom") {
		return fmt.Errorf("重复规则类型无效")
	}
	if value := rule["interval"]; value != nil {
		n, ok := integer(value)
		if !ok || n <= 0 || n > 10000 {
			return fmt.Errorf("重复间隔无效")
		}
	}
	overrideValue, err := parseJSONValue([]byte(data["occurrenceOverrides"].(string)))
	if err != nil {
		return fmt.Errorf("重复事项覆盖无效")
	}
	overrides, ok := overrideValue.(map[string]any)
	if !ok {
		return fmt.Errorf("重复事项覆盖必须为对象")
	}
	for date, value := range overrides {
		if !validDate(date) {
			return fmt.Errorf("重复事项覆盖日期无效")
		}
		item, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("重复事项覆盖必须为对象")
		}
		if value := item["status"]; value != nil && !oneOf(value, "todo", "done", "failed") {
			return fmt.Errorf("重复事项状态无效")
		}
		for _, key := range []string{"title", "notes", "failureReason"} {
			if value := item[key]; value != nil && !isString(value) {
				return fmt.Errorf("重复事项文本无效")
			}
		}
		if value := item["categoryId"]; value != nil {
			text, ok := value.(string)
			if !ok || !idPattern.MatchString(text) {
				return fmt.Errorf("重复事项分类无效")
			}
		}
	}
	return nil
}

func inputRecordID(data map[string]any) string {
	anniversaryID, ok1 := data["anniversaryId"].(string)
	date, ok2 := data["date"].(string)
	if !ok1 || !ok2 {
		return ""
	}
	return anniversaryID + "@" + date
}

func validDate(value string) bool {
	if !datePattern.MatchString(value) {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validJSONString(value any) bool {
	text, ok := value.(string)
	return ok && json.Valid([]byte(text))
}

func checkText(data map[string]any, first string, firstMax int, firstRequired bool, rest ...any) error {
	checks := []any{first, firstMax, firstRequired}
	checks = append(checks, rest...)
	for i := 0; i < len(checks); i += 3 {
		name := checks[i].(string)
		max := checks[i+1].(int)
		required := checks[i+2].(bool)
		value := data[name]
		if value == nil && !required {
			continue
		}
		text, ok := value.(string)
		if !ok || (required && strings.TrimSpace(text) == "") || len([]rune(text)) > max {
			return fmt.Errorf("字段 %s 格式无效", name)
		}
	}
	return nil
}

func isString(value any) bool { _, ok := value.(string); return ok }

func oneOf(value any, choices ...string) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	for _, choice := range choices {
		if text == choice {
			return true
		}
	}
	return false
}

func validateSetting(id string, value any, deleted bool) error {
	switch id {
	case "show_completed.tasks", "show_completed.week", "show_completed.month":
		if !deleted {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("显示偏好必须为布尔值")
			}
		}
	case "desktop.last_task_category":
		if !deleted {
			if text, ok := value.(string); !ok || text != "" && !idPattern.MatchString(text) {
				return fmt.Errorf("默认分类无效")
			}
		}
	case "desktop.last_task_priority":
		if !deleted && !oneOf(value, "urgent_important", "important_not_urgent", "urgent_not_important", "not_urgent_not_important") {
			return fmt.Errorf("默认优先级无效")
		}
	case "mobile.collapsed_groups":
		if !deleted {
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("分组偏好必须为数组")
			}
		}
	case "mobile.profile", "mobile.new_task_defaults":
		if !deleted {
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("偏好值必须为对象")
			}
		}
	default:
		return fmt.Errorf("未知设置标识")
	}
	return nil
}

func integer(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	parsed, err := number.Int64()
	return parsed, err == nil
}

func photoStrings(value any, limit int64) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	items, ok := value.([]any)
	if !ok || len(items) > 4 {
		return nil, fmt.Errorf("纪念日照片格式无效")
	}
	photos := make([]string, 0, len(items))
	var size int64
	for _, item := range items {
		photo, ok := item.(string)
		if !ok || !strings.HasPrefix(photo, "data:image/jpeg;base64,") {
			return nil, fmt.Errorf("纪念日照片必须为 JPEG data URL")
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(strings.TrimPrefix(photo, "data:image/jpeg;base64,"))
		if err != nil || len(decoded) < 32 || len(decoded) > 300*1024 || decoded[0] != 0xff || decoded[1] != 0xd8 || decoded[len(decoded)-2] != 0xff || decoded[len(decoded)-1] != 0xd9 {
			return nil, fmt.Errorf("纪念日照片数据或大小无效")
		}
		size += int64(len(photo))
		photos = append(photos, photo)
	}
	if size > limit {
		return nil, fmt.Errorf("纪念日照片超出体积限制")
	}
	return photos, nil
}
