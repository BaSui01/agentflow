// 结构化包在JSON Schema验证下提供结构化输出支持.
package structured

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// SchemaGenerator利用反射从Go类型生成了JSON Schema.
type SchemaGenerator struct {
	// 正在处理处理递归类型的访问音轨类型
	visited map[reflect.Type]bool
}

// NewSchemaGenerator创建了一个新的SchemaGenerator实例.
func NewSchemaGenerator() *SchemaGenerator {
	return &SchemaGenerator{
		visited: make(map[reflect.Type]bool),
	}
}

// 生成Schema从Go类型生成一个JSON Schema.
// 它支持结构、切片、地图、指针和基本类型。
// Struct字段可以使用"json"标记来表示字段名称和"jsonschema"标记来表示验证限制.
//
// 支持的 jsonschema 标签选项 :
//   - 所需:按需要标出字段
//   - enum=a,b,c: enum值
//   - 最小=0:数字的最低值
//   - 最大值=100:数字的最大值
//   - minLength=1:最小字符串长度
//   - 最大Length=100:最大字符串长度
//   -图案QQ[a-z]+美元:字符串的正则图案
//   -格式=电子邮件:字符串格式(电子邮件、uri、uuid、日期-时间等)
//   - 分钟项目=1:最小数组项目
//   - 最大项目=10:最大数组项目
//   - 说明=: 实地说明
//   - 默认=.: 默认值
func (g *SchemaGenerator) GenerateSchema(t reflect.Type) (*JSONSchema, error) {
	// 为每个顶级呼叫重置访问的地图
	g.visited = make(map[reflect.Type]bool)
	return g.generateSchema(t)
}

// 生成Schema是内部递归执行。
func (g *SchemaGenerator) generateSchema(t reflect.Type) (*JSONSchema, error) {
	// 处理零类型
	if t == nil {
		return nil, fmt.Errorf("cannot generate schema for nil type")
	}

	// 引用指针类型
	if t.Kind() == reflect.Ptr {
		return g.generateSchema(t.Elem())
	}

	// 检查递归类型
	if g.visited[t] {
		// 返回递归类型的参考占位符
		return &JSONSchema{Type: TypeObject}, nil
	}

	switch t.Kind() {
	case reflect.String:
		return NewStringSchema(), nil

	case reflect.Bool:
		return NewBooleanSchema(), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return NewIntegerSchema(), nil

	case reflect.Float32, reflect.Float64:
		return NewNumberSchema(), nil

	case reflect.Slice, reflect.Array:
		return g.generateArraySchema(t)

	case reflect.Map:
		return g.generateMapSchema(t)

	case reflect.Struct:
		return g.generateStructSchema(t)

	case reflect.Interface:
		// 接口QQ 映射到任意类型
		return &JSONSchema{}, nil

	default:
		return nil, fmt.Errorf("unsupported type: %s", t.Kind())
	}
}

// 生成切片/阵列类型的ArraySchema生成子图。
func (g *SchemaGenerator) generateArraySchema(t reflect.Type) (*JSONSchema, error) {
	elemSchema, err := g.generateSchema(t.Elem())
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for array element: %w", err)
	}
	return NewArraySchema(elemSchema), nil
}

// 生成MapSchema为地图类型生成了计划。
func (g *SchemaGenerator) generateMapSchema(t reflect.Type) (*JSONSchema, error) {
	// 映射为带有额外特性的对象
	valueSchema, err := g.generateSchema(t.Elem())
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema for map value: %w", err)
	}

	schema := NewObjectSchema()
	schema.AdditionalProperties = &AdditionalProperties{
		Allowed: true,
		Schema:  valueSchema,
	}
	return schema, nil
}

// 生成 StructSchema 为struct 类型生成 schema。
func (g *SchemaGenerator) generateStructSchema(t reflect.Type) (*JSONSchema, error) {
	// 标记为访问处理递归类型
	g.visited[t] = true
	defer func() { g.visited[t] = false }()

	schema := NewObjectSchema()
	schema.Type = TypeObject

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过未导出字段
		if !field.IsExported() {
			continue
		}

		// 从 json 标签中获取字段名称或使用字段名称
		fieldName := getJSONFieldName(field)
		if fieldName == "-" {
			continue // Skip fields with json:"-"
		}

		// 生成字段类型的计划
		fieldSchema, err := g.generateSchema(field.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to generate schema for field %s: %w", field.Name, err)
		}

		// 应用 jsonschema 标签限制
		if err := applyJSONSchemaTag(fieldSchema, field); err != nil {
			return nil, fmt.Errorf("failed to apply jsonschema tag for field %s: %w", field.Name, err)
		}

		// 检查是否需要字段
		if isFieldRequired(field) {
			schema.Required = append(schema.Required, fieldName)
		}

		schema.Properties[fieldName] = fieldSchema
	}

	return schema, nil
}

// 获取 JSON 字段 名称从 json 标签中提取字段名称或返回 struct 字段名称。
func getJSONFieldName(field reflect.StructField) string {
	jsonTag := field.Tag.Get("json")
	if jsonTag == "" {
		return field.Name
	}

	// 解析 json 标签(格式 : “ 名称、 选项” )
	parts := strings.Split(jsonTag, ",")
	name := parts[0]

	if name == "" {
		return field.Name
	}

	return name
}

// 是通过 jsonschema 标记按要求标记一个字段时, 是 Field Required check 。
func isFieldRequired(field reflect.StructField) bool {
	jsonschemaTag := field.Tag.Get("jsonschema")
	if jsonschemaTag == "" {
		return false
	}

	options := parseTagOptions(jsonschemaTag)
	_, required := options["required"]
	return required
}

// 应用 JSONSchemaTag 将 jsonschema 标记限制应用到一个计划。
func applyJSONSchemaTag(schema *JSONSchema, field reflect.StructField) error {
	jsonschemaTag := field.Tag.Get("jsonschema")
	if jsonschemaTag == "" {
		return nil
	}

	options := parseTagOptions(jsonschemaTag)

	// 应用描述
	if desc, ok := options["description"]; ok {
		schema.Description = desc
	}

	// 应用默认值
	if def, ok := options["default"]; ok {
		schema.Default = parseDefaultValue(def, field.Type)
	}

	// 应用 enum 值
	if enumStr, ok := options["enum"]; ok {
		enumValues := strings.Split(enumStr, ",")
		schema.Enum = make([]any, len(enumValues))
		for i, v := range enumValues {
			schema.Enum[i] = strings.TrimSpace(v)
		}
	}

	// 应用字符串限制
	if minLen, ok := options["minLength"]; ok {
		if v, err := strconv.Atoi(minLen); err == nil {
			schema.MinLength = &v
		}
	}
	if maxLen, ok := options["maxLength"]; ok {
		if v, err := strconv.Atoi(maxLen); err == nil {
			schema.MaxLength = &v
		}
	}
	if pattern, ok := options["pattern"]; ok {
		schema.Pattern = pattern
	}
	if format, ok := options["format"]; ok {
		schema.Format = StringFormat(format)
	}

	// 应用数字限制
	if min, ok := options["minimum"]; ok {
		if v, err := strconv.ParseFloat(min, 64); err == nil {
			schema.Minimum = &v
		}
	}
	if max, ok := options["maximum"]; ok {
		if v, err := strconv.ParseFloat(max, 64); err == nil {
			schema.Maximum = &v
		}
	}

	// 应用数组限制
	if minItems, ok := options["minItems"]; ok {
		if v, err := strconv.Atoi(minItems); err == nil {
			schema.MinItems = &v
		}
	}
	if maxItems, ok := options["maxItems"]; ok {
		if v, err := strconv.Atoi(maxItems); err == nil {
			schema.MaxItems = &v
		}
	}

	return nil
}

// 解析 Tag 选项将 jsonschema 标记字符串解为选项地图。
// 格式 : “ 选项 1, 选项2 = 值 2, 选项3 = 值 3 ”
func parseTagOptions(tag string) map[string]string {
	options := make(map[string]string)
	if tag == "" {
		return options
	}

	parts := splitTagParts(tag)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 检查密钥=值格式
		if idx := strings.Index(part, "="); idx > 0 {
			key := part[:idx]
			value := part[idx+1:]
			options[key] = value
		} else {
			// Boolean 选项( 例如“ 必需的 ” )
			options[part] = ""
		}
	}

	return options
}

// 拆分 TagParts 用逗号分割标签字符串, 但尊重包含逗号的值
// 在enum值之内(例如,“enum=a,b,c”应当将“a,b,c”保留在一起)。
// 逻辑:在看到"="后,我们处于一个值. 一个逗号只有在
// 下一段看起来像一个新的键(在任何特殊字符之前没有“=”的缩写,
// 或者是已知的布尔选项,比如"必需".
func splitTagParts(tag string) []string {
	var parts []string
	var current strings.Builder
	inValue := false

	// 已知没有"="的布尔选项
	knownBoolOptions := map[string]bool{
		"required": true,
	}

	for i := 0; i < len(tag); i++ {
		ch := tag[i]

		if ch == '=' {
			inValue = true
			current.WriteByte(ch)
		} else if ch == ',' && !inValue {
			parts = append(parts, current.String())
			current.Reset()
		} else if ch == ',' && inValue {
			// 检查此逗号是否是 enum 值的一部分, 或是分隔选项
			// 向前看,看下半段看是不是新选择
			remaining := tag[i+1:]

			// 查找下段( 上到下个逗号或结尾)
			nextComma := strings.Index(remaining, ",")
			var nextSegment string
			if nextComma >= 0 {
				nextSegment = remaining[:nextComma]
			} else {
				nextSegment = remaining
			}
			nextSegment = strings.TrimSpace(nextSegment)

			// 检查下一段是否为已知布尔选项
			if knownBoolOptions[nextSegment] {
				parts = append(parts, current.String())
				current.Reset()
				inValue = false
				continue
			}

			// 检查下一段是否看起来像一个键=值选项
			// 它应该有"=",而"="前面的部分应该是一个有效的密钥(缩写)
			if eqIdx := strings.Index(nextSegment, "="); eqIdx > 0 {
				potentialKey := nextSegment[:eqIdx]
				// 有效密钥: 字母数字, 无空格
				isValidKey := true
				for _, c := range potentialKey {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
						isValidKey = false
						break
					}
				}
				if isValidKey {
					parts = append(parts, current.String())
					current.Reset()
					inValue = false
					continue
				}
			}

			// 此逗号是当前值的一部分( 如 enum 值)
			current.WriteByte(ch)
		} else {
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// 解析DefaultValue将默认值字符串分解为适当的类型。
func parseDefaultValue(value string, t reflect.Type) any {
	// 引用指针
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return value
	case reflect.Bool:
		return value == "true"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			return v
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if v, err := strconv.ParseUint(value, 10, 64); err == nil {
			return v
		}
	case reflect.Float32, reflect.Float64:
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			return v
		}
	}
	return value
}

// 生成SchemaFromValue 从一个值的类型生成一个JSON Schema.
// 这是一个从一个值中提取类型的便利函数.
func (g *SchemaGenerator) GenerateSchemaFromValue(v any) (*JSONSchema, error) {
	if v == nil {
		return nil, fmt.Errorf("cannot generate schema from nil value")
	}
	return g.GenerateSchema(reflect.TypeOf(v))
}
