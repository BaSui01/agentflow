package structured

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// SchemaValidator对照一个JSONSchema验证了JSON数据.
type SchemaValidator interface {
	Validate(data []byte, schema *JSONSchema) error
}

// ParseError 代表着与字段路径的校验错误 。
type ParseError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// 执行错误接口出错 。
func (e *ParseError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// 校验错误( Errors) 代表多个校验错误.
type ValidationErrors struct {
	Errors []ParseError `json:"errors"`
}

// 执行错误接口出错 。
func (e *ValidationErrors) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("validation failed with %d errors: %s", len(e.Errors), strings.Join(msgs, "; "))
}

// 默认变量是 SchemaValidator 的默认执行.
type DefaultValidator struct {
	// 格式变异器持有自定义格式验证器
	formatValidators map[StringFormat]func(string) bool
}

// NewValidator 创建了带有内置格式验证符的新默认变量.
func NewValidator() *DefaultValidator {
	v := &DefaultValidator{
		formatValidators: make(map[StringFormat]func(string) bool),
	}
	v.registerBuiltinFormats()
	return v
}

// 注册BuiltinFormats 注册内置格式验证器。
func (v *DefaultValidator) registerBuiltinFormats() {
	// 电子邮件格式
	v.formatValidators[FormatEmail] = func(s string) bool {
		// 简单的电子邮件正则x
		pattern := `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// URI 格式
	v.formatValidators[FormatURI] = func(s string) bool {
		pattern := `^[a-zA-Z][a-zA-Z0-9+.-]*://`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// UUID 格式
	v.formatValidators[FormatUUID] = func(s string) bool {
		pattern := `^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// 日期格式(ISO 8601)
	v.formatValidators[FormatDateTime] = func(s string) bool {
		pattern := `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(.\d+)?(Z|[+-]\d{2}:\d{2})?$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// 日期格式
	v.formatValidators[FormatDate] = func(s string) bool {
		pattern := `^\d{4}-\d{2}-\d{2}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// 时间格式
	v.formatValidators[FormatTime] = func(s string) bool {
		pattern := `^\d{2}:\d{2}:\d{2}(.\d+)?(Z|[+-]\d{2}:\d{2})?$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// IPv4 格式
	v.formatValidators[FormatIPv4] = func(s string) bool {
		pattern := `^(\d{1,3}\.){3}\d{1,3}$`
		matched, _ := regexp.MatchString(pattern, s)
		if !matched {
			return false
		}
		parts := strings.Split(s, ".")
		for _, part := range parts {
			var num int
			fmt.Sscanf(part, "%d", &num)
			if num < 0 || num > 255 {
				return false
			}
		}
		return true
	}

	// IPv6 格式
	v.formatValidators[FormatIPv6] = func(s string) bool {
		pattern := `^([0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}$|^::$|^([0-9a-fA-F]{1,4}:)*:([0-9a-fA-F]{1,4}:)*[0-9a-fA-F]{1,4}$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched
	}

	// 主机名格式
	v.formatValidators[FormatHostname] = func(s string) bool {
		pattern := `^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`
		matched, _ := regexp.MatchString(pattern, s)
		return matched && len(s) <= 253
	}
}

// RegisterFormat 注册自定义格式验证符 。
func (v *DefaultValidator) RegisterFormat(format StringFormat, validator func(string) bool) {
	v.formatValidators[format] = validator
}

// 校验组对照一个计划验证JSON数据.
func (v *DefaultValidator) Validate(data []byte, schema *JSONSchema) error {
	if schema == nil {
		return nil
	}

	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return &ValidationErrors{
			Errors: []ParseError{{Path: "", Message: fmt.Sprintf("invalid JSON: %v", err)}},
		}
	}

	var errors []ParseError
	v.validateValue(value, schema, "", &errors)

	if len(errors) > 0 {
		return &ValidationErrors{Errors: errors}
	}
	return nil
}

// 验证Value 在给定路径上对照一个计划验证一个值。
func (v *DefaultValidator) validateValue(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if schema == nil {
		return
	}

	// 先检查康斯特
	if schema.Const != nil {
		if !v.equalValues(value, schema.Const) {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value must be %v", schema.Const),
			})
		}
		return
	}

	// 检查enum
	if len(schema.Enum) > 0 {
		found := false
		for _, enumVal := range schema.Enum {
			if v.equalValues(value, enumVal) {
				found = true
				break
			}
		}
		if !found {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value must be one of: %v", schema.Enum),
			})
		}
	}

	// 根据类型验证
	if schema.Type != "" {
		v.validateType(value, schema, path, errors)
	}
}

// 验证 Type 对照预期类型验证一个值 。
func (v *DefaultValidator) validateType(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	switch schema.Type {
	case TypeString:
		v.validateString(value, schema, path, errors)
	case TypeNumber:
		v.validateNumber(value, schema, path, errors)
	case TypeInteger:
		v.validateInteger(value, schema, path, errors)
	case TypeBoolean:
		v.validateBoolean(value, schema, path, errors)
	case TypeNull:
		v.validateNull(value, schema, path, errors)
	case TypeObject:
		v.validateObject(value, schema, path, errors)
	case TypeArray:
		v.validateArray(value, schema, path, errors)
	}
}

// 验证 String 验证字符串值。
func (v *DefaultValidator) validateString(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	str, ok := value.(string)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected string, got %T", value),
		})
		return
	}

	// 检查分钟
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("string length %d is less than minimum %d", len(str), *schema.MinLength),
		})
	}

	// 检查最大
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("string length %d exceeds maximum %d", len(str), *schema.MaxLength),
		})
	}

	// 检查图案
	if schema.Pattern != "" {
		matched, err := regexp.MatchString(schema.Pattern, str)
		if err != nil {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("invalid pattern %q: %v", schema.Pattern, err),
			})
		} else if !matched {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("string does not match pattern %q", schema.Pattern),
			})
		}
	}

	// 检查格式
	if schema.Format != "" {
		if validator, ok := v.formatValidators[schema.Format]; ok {
			if !validator(str) {
				*errors = append(*errors, ParseError{
					Path:    path,
					Message: fmt.Sprintf("string does not match format %q", schema.Format),
				})
			}
		}
	}
}

// 数字验证一个数字值。
func (v *DefaultValidator) validateNumber(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	num, ok := v.toFloat64(value)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected number, got %T", value),
		})
		return
	}

	v.validateNumericConstraints(num, schema, path, errors)
}

// 验证整数 。
func (v *DefaultValidator) validateInteger(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	num, ok := v.toFloat64(value)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected integer, got %T", value),
		})
		return
	}

	// 检查它是否是整数
	if num != math.Trunc(num) {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected integer, got %v", num),
		})
		return
	}

	v.validateNumericConstraints(num, schema, path, errors)
}

// 验证 Numeric Constructions 验证数字限制 。
func (v *DefaultValidator) validateNumericConstraints(num float64, schema *JSONSchema, path string, errors *[]ParseError) {
	// 检查最小值
	if schema.Minimum != nil && num < *schema.Minimum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v is less than minimum %v", num, *schema.Minimum),
		})
	}

	// 检查最大值
	if schema.Maximum != nil && num > *schema.Maximum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v exceeds maximum %v", num, *schema.Maximum),
		})
	}

	// 选中最小值
	if schema.ExclusiveMinimum != nil && num <= *schema.ExclusiveMinimum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v must be greater than %v", num, *schema.ExclusiveMinimum),
		})
	}

	// 检查独有的Maximum
	if schema.ExclusiveMaximum != nil && num >= *schema.ExclusiveMaximum {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("value %v must be less than %v", num, *schema.ExclusiveMaximum),
		})
	}

	// 检查多处
	if schema.MultipleOf != nil && *schema.MultipleOf != 0 {
		quotient := num / *schema.MultipleOf
		if quotient != math.Trunc(quotient) {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("value %v is not a multiple of %v", num, *schema.MultipleOf),
			})
		}
	}
}

// 验证 Boolean 验证布尔值 。
func (v *DefaultValidator) validateBoolean(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if _, ok := value.(bool); !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected boolean, got %T", value),
		})
	}
}

// 验证 Null 验证一个无效值。
func (v *DefaultValidator) validateNull(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	if value != nil {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected null, got %T", value),
		})
	}
}

// 验证对象验证对象值。
func (v *DefaultValidator) validateObject(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	obj, ok := value.(map[string]any)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected object, got %T", value),
		})
		return
	}

	// 检查需要的字段
	for _, req := range schema.Required {
		val, exists := obj[req]
		if !exists {
			*errors = append(*errors, ParseError{
				Path:    v.joinPath(path, req),
				Message: "required field is missing",
			})
		} else if val == nil {
			*errors = append(*errors, ParseError{
				Path:    v.joinPath(path, req),
				Message: "required field must not be null",
			})
		}
	}

	// 检查 minProperties
	if schema.MinProperties != nil && len(obj) < *schema.MinProperties {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("object has %d properties, minimum is %d", len(obj), *schema.MinProperties),
		})
	}

	// 检查最大收益
	if schema.MaxProperties != nil && len(obj) > *schema.MaxProperties {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("object has %d properties, maximum is %d", len(obj), *schema.MaxProperties),
		})
	}

	// 校验属性
	for propName, propValue := range obj {
		propPath := v.joinPath(path, propName)

		// 检查是否在计划中定义属性
		if propSchema, ok := schema.Properties[propName]; ok {
			v.validateValue(propValue, propSchema, propPath, errors)
		} else if schema.AdditionalProperties != nil {
			// 检查额外财产
			if !schema.AdditionalProperties.Allowed && schema.AdditionalProperties.Schema == nil {
				*errors = append(*errors, ParseError{
					Path:    propPath,
					Message: "additional property not allowed",
				})
			} else if schema.AdditionalProperties.Schema != nil {
				v.validateValue(propValue, schema.AdditionalProperties.Schema, propPath, errors)
			}
		}

		// 检查模式
		for pattern, patternSchema := range schema.PatternProperties {
			matched, err := regexp.MatchString(pattern, propName)
			if err == nil && matched {
				v.validateValue(propValue, patternSchema, propPath, errors)
			}
		}
	}

	// 验证属性Names
	if schema.PropertyNames != nil {
		for propName := range obj {
			v.validateValue(propName, schema.PropertyNames, v.joinPath(path, propName), errors)
		}
	}
}

// 验证阵列验证一个数组值。
func (v *DefaultValidator) validateArray(value any, schema *JSONSchema, path string, errors *[]ParseError) {
	arr, ok := value.([]any)
	if !ok {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("expected array, got %T", value),
		})
		return
	}

	// 检查分钟项目
	if schema.MinItems != nil && len(arr) < *schema.MinItems {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("array has %d items, minimum is %d", len(arr), *schema.MinItems),
		})
	}

	// 检查最大项目
	if schema.MaxItems != nil && len(arr) > *schema.MaxItems {
		*errors = append(*errors, ParseError{
			Path:    path,
			Message: fmt.Sprintf("array has %d items, maximum is %d", len(arr), *schema.MaxItems),
		})
	}

	// 检查独有的项目
	if schema.UniqueItems != nil && *schema.UniqueItems {
		seen := make(map[string]bool)
		for i, item := range arr {
			key := v.valueKey(item)
			if seen[key] {
				*errors = append(*errors, ParseError{
					Path:    fmt.Sprintf("%s[%d]", path, i),
					Message: "duplicate item in array with uniqueItems constraint",
				})
			}
			seen[key] = true
		}
	}

	// 验证项目
	if schema.Items != nil {
		for i, item := range arr {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			v.validateValue(item, schema.Items, itemPath, errors)
		}
	}

	// 校验前缀项目
	if len(schema.PrefixItems) > 0 {
		for i, prefixSchema := range schema.PrefixItems {
			if i < len(arr) {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				v.validateValue(arr[i], prefixSchema, itemPath, errors)
			}
		}
	}

	// 校验包含
	if schema.Contains != nil {
		containsCount := 0
		for _, item := range arr {
			var itemErrors []ParseError
			v.validateValue(item, schema.Contains, "", &itemErrors)
			if len(itemErrors) == 0 {
				containsCount++
			}
		}

		minContains := 1
		if schema.MinContains != nil {
			minContains = *schema.MinContains
		}

		if containsCount < minContains {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("array must contain at least %d matching items, found %d", minContains, containsCount),
			})
		}

		if schema.MaxContains != nil && containsCount > *schema.MaxContains {
			*errors = append(*errors, ParseError{
				Path:    path,
				Message: fmt.Sprintf("array must contain at most %d matching items, found %d", *schema.MaxContains, containsCount),
			})
		}
	}
}

// toFloat64 将一个值转换为浮动64。
func (v *DefaultValidator) toFloat64(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// 平等价值比较两个平等价值。
func (v *DefaultValidator) equalValues(a, b any) bool {
	// 处理数字比较
	aNum, aIsNum := v.toFloat64(a)
	bNum, bIsNum := v.toFloat64(b)
	if aIsNum && bIsNum {
		return aNum == bNum
	}

	// 处理字符串比较
	aStr, aIsStr := a.(string)
	bStr, bIsStr := b.(string)
	if aIsStr && bIsStr {
		return aStr == bStr
	}

	// 处理布尔比较
	aBool, aIsBool := a.(bool)
	bBool, bIsBool := b.(bool)
	if aIsBool && bIsBool {
		return aBool == bBool
	}

	// 处理
	if a == nil && b == nil {
		return true
	}

	// 对复杂类型使用 JSON 序列化
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

// 加入 Path 加入路径段 。
func (v *DefaultValidator) joinPath(base, segment string) string {
	if base == "" {
		return segment
	}
	return base + "." + segment
}

// 值 Key 生成一个值的独有密钥(用于唯一的项目检查) 。
func (v *DefaultValidator) valueKey(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}
