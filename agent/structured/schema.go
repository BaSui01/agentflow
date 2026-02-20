package structured

import (
	"encoding/json"
	"fmt"
)

// SchemaType代表JSON Schema类型.
type SchemaType string

const (
	TypeString  SchemaType = "string"
	TypeNumber  SchemaType = "number"
	TypeInteger SchemaType = "integer"
	TypeBoolean SchemaType = "boolean"
	TypeNull    SchemaType = "null"
	TypeObject  SchemaType = "object"
	TypeArray   SchemaType = "array"
)

// StringFormat 代表常见的字符串格式限制.
type StringFormat string

const (
	FormatDateTime StringFormat = "date-time"
	FormatDate     StringFormat = "date"
	FormatTime     StringFormat = "time"
	FormatEmail    StringFormat = "email"
	FormatURI      StringFormat = "uri"
	FormatUUID     StringFormat = "uuid"
	FormatHostname StringFormat = "hostname"
	FormatIPv4     StringFormat = "ipv4"
	FormatIPv6     StringFormat = "ipv6"
)

// JSONSchema代表了JSON Schema的定义.
// 它支持嵌入对象,阵列,enum以及各种验证约束.
type JSONSchema struct {
	// 核心元数据
	Schema      string `json:"$schema,omitempty"`
	ID          string `json:"$id,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`

	// 类型定义
	Type SchemaType `json:"type,omitempty"`

	// 对象属性
	Properties           map[string]*JSONSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *AdditionalProperties  `json:"additionalProperties,omitempty"`
	MinProperties        *int                   `json:"minProperties,omitempty"`
	MaxProperties        *int                   `json:"maxProperties,omitempty"`
	PatternProperties    map[string]*JSONSchema `json:"patternProperties,omitempty"`
	PropertyNames        *JSONSchema            `json:"propertyNames,omitempty"`

	// 矩阵项目
	Items       *JSONSchema   `json:"items,omitempty"`
	PrefixItems []*JSONSchema `json:"prefixItems,omitempty"`
	Contains    *JSONSchema   `json:"contains,omitempty"`
	MinItems    *int          `json:"minItems,omitempty"`
	MaxItems    *int          `json:"maxItems,omitempty"`
	UniqueItems *bool         `json:"uniqueItems,omitempty"`
	MinContains *int          `json:"minContains,omitempty"`
	MaxContains *int          `json:"maxContains,omitempty"`

	// 凸起和凸起
	Enum  []any `json:"enum,omitempty"`
	Const any   `json:"const,omitempty"`

	// 字符串制约
	MinLength *int         `json:"minLength,omitempty"`
	MaxLength *int         `json:"maxLength,omitempty"`
	Pattern   string       `json:"pattern,omitempty"`
	Format    StringFormat `json:"format,omitempty"`

	// 数字限制
	Minimum          *float64 `json:"minimum,omitempty"`
	Maximum          *float64 `json:"maximum,omitempty"`
	ExclusiveMinimum *float64 `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum *float64 `json:"exclusiveMaximum,omitempty"`
	MultipleOf       *float64 `json:"multipleOf,omitempty"`

	// 默认值
	Default any `json:"default,omitempty"`

	// 实例
	Examples []any `json:"examples,omitempty"`

	// 组成关键词
	AllOf []*JSONSchema `json:"allOf,omitempty"`
	AnyOf []*JSONSchema `json:"anyOf,omitempty"`
	OneOf []*JSONSchema `json:"oneOf,omitempty"`
	Not   *JSONSchema   `json:"not,omitempty"`

	// 有条件的关键词
	If   *JSONSchema `json:"if,omitempty"`
	Then *JSONSchema `json:"then,omitempty"`
	Else *JSONSchema `json:"else,omitempty"`

	// 重复使用的定义
	Defs map[string]*JSONSchema `json:"$defs,omitempty"`
}

// 附加Properties 代表额外的Properties字段,可以是
// 或布林克或计划。
type AdditionalProperties struct {
	Allowed bool
	Schema  *JSONSchema
}

// JSON警长执行JSON。 增产元帅.
func (ap *AdditionalProperties) MarshalJSON() ([]byte, error) {
	if ap == nil {
		return json.Marshal(nil)
	}
	if ap.Schema != nil {
		return json.Marshal(ap.Schema)
	}
	return json.Marshal(ap.Allowed)
}

// UnmarshalJSON 执行json。 (原始内容存档于2018-10-21). Unmarshaler for Purposities.
func (ap *AdditionalProperties) UnmarshalJSON(data []byte) error {
	// 先试试布尔
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		ap.Allowed = b
		ap.Schema = nil
		return nil
	}

	// 尝试计划
	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err == nil {
		ap.Allowed = true
		ap.Schema = &schema
		return nil
	}

	return fmt.Errorf("additionalProperties must be boolean or schema")
}

// NewSchema创建了具有指定类型的新的JSONSchema.
func NewSchema(t SchemaType) *JSONSchema {
	return &JSONSchema{Type: t}
}

// 新对象计划创建了新对象计划.
func NewObjectSchema() *JSONSchema {
	return &JSONSchema{
		Type:       TypeObject,
		Properties: make(map[string]*JSONSchema),
	}
}

// NewArraySchema 创建一个带有指定项目 schema 的新阵列计划.
func NewArraySchema(items *JSONSchema) *JSONSchema {
	return &JSONSchema{
		Type:  TypeArray,
		Items: items,
	}
}

// NewStringSchema创建了一个新的字符串Schema.
func NewStringSchema() *JSONSchema {
	return &JSONSchema{Type: TypeString}
}

// NumberSchema创建了新的编号Schema.
func NewNumberSchema() *JSONSchema {
	return &JSONSchema{Type: TypeNumber}
}

// NewIntegerSchema创建了新的整数计划.
func NewIntegerSchema() *JSONSchema {
	return &JSONSchema{Type: TypeInteger}
}

// NewBooleanSchema 创建了一个新的布尔计划.
func NewBooleanSchema() *JSONSchema {
	return &JSONSchema{Type: TypeBoolean}
}

// NewEnumSchema 创建一个带有指定值的新的enumschema.
func NewEnumSchema(values ...any) *JSONSchema {
	return &JSONSchema{Enum: values}
}

// 使用 Title 设置标题并返回用于连锁的图案 。
func (s *JSONSchema) WithTitle(title string) *JSONSchema {
	s.Title = title
	return s
}

// 使用Description 设置描述并返回链路的策略 。
func (s *JSONSchema) WithDescription(desc string) *JSONSchema {
	s.Description = desc
	return s
}

// With Default 设置了默认值并返回用于链路的图案.
func (s *JSONSchema) WithDefault(def any) *JSONSchema {
	s.Default = def
	return s
}

// 以实例设置示例并返回链条的图案。
func (s *JSONSchema) WithExamples(examples ...any) *JSONSchema {
	s.Examples = examples
	return s
}

// 添加 Property 为对象计划添加属性。
func (s *JSONSchema) AddProperty(name string, prop *JSONSchema) *JSONSchema {
	if s.Properties == nil {
		s.Properties = make(map[string]*JSONSchema)
	}
	s.Properties[name] = prop
	return s
}

// 添加所需的字段名称到对象计划 。
func (s *JSONSchema) AddRequired(names ...string) *JSONSchema {
	s.Required = append(s.Required, names...)
	return s
}

// 通过MinLength设定了字符串计划的最低长度.
func (s *JSONSchema) WithMinLength(min int) *JSONSchema {
	s.MinLength = &min
	return s
}

// 通过MaxLength设定了字符串计划的最大长度.
func (s *JSONSchema) WithMaxLength(max int) *JSONSchema {
	s.MaxLength = &max
	return s
}

// WithPattern设定了字符串计划的模式.
func (s *JSONSchema) WithPattern(pattern string) *JSONSchema {
	s.Pattern = pattern
	return s
}

// 用Format设置字符串计划的格式。
func (s *JSONSchema) WithFormat(format StringFormat) *JSONSchema {
	s.Format = format
	return s
}

// 以最小值设定数值方案的最低值。
func (s *JSONSchema) WithMinimum(min float64) *JSONSchema {
	s.Minimum = &min
	return s
}

// 使用 Maximum 设置了数值计数的最大值 。
func (s *JSONSchema) WithMaximum(max float64) *JSONSchema {
	s.Maximum = &max
	return s
}

// 以排除最小值设置数值计的专属最小值。
func (s *JSONSchema) WithExclusiveMinimum(min float64) *JSONSchema {
	s.ExclusiveMinimum = &min
	return s
}

// With Exclusive Maximum 为数值计设置了独有的最大值.
func (s *JSONSchema) WithExclusiveMaximum(max float64) *JSONSchema {
	s.ExclusiveMaximum = &max
	return s
}

// MultipleOf 设置了数值图的多 Of 约束。
func (s *JSONSchema) WithMultipleOf(val float64) *JSONSchema {
	s.MultipleOf = &val
	return s
}

// With Min Projects 为阵列计划设定最小项目 。
func (s *JSONSchema) WithMinItems(min int) *JSONSchema {
	s.MinItems = &min
	return s
}

// With Max Projects为阵列计划设定了最大项目.
func (s *JSONSchema) WithMaxItems(max int) *JSONSchema {
	s.MaxItems = &max
	return s
}

// 独有的项目设置了数组计划唯一的项目限制 。
func (s *JSONSchema) WithUniqueItems(unique bool) *JSONSchema {
	s.UniqueItems = &unique
	return s
}

// 与 MinProperties 设定对象方案的最低属性 。
func (s *JSONSchema) WithMinProperties(min int) *JSONSchema {
	s.MinProperties = &min
	return s
}

// 与MaxProperties设定对象计划的最大属性.
func (s *JSONSchema) WithMaxProperties(max int) *JSONSchema {
	s.MaxProperties = &max
	return s
}

// 附加物业设置附加物业约束.
func (s *JSONSchema) WithAdditionalProperties(allowed bool) *JSONSchema {
	s.AdditionalProperties = &AdditionalProperties{Allowed: allowed}
	return s
}

// 附加的PropertiesSchema将附加的Properties设定为一个 schema.
func (s *JSONSchema) WithAdditionalPropertiesSchema(schema *JSONSchema) *JSONSchema {
	s.AdditionalProperties = &AdditionalProperties{Allowed: true, Schema: schema}
	return s
}

// 用 Enum 设置 enum 值 。
func (s *JSONSchema) WithEnum(values ...any) *JSONSchema {
	s.Enum = values
	return s
}

// 用 Const 设置 Const 值 。
func (s *JSONSchema) WithConst(value any) *JSONSchema {
	s.Const = value
	return s
}

// 克隆人创造出一个深层的复制图案.
func (s *JSONSchema) Clone() *JSONSchema {
	if s == nil {
		return nil
	}

	clone := &JSONSchema{
		Schema:      s.Schema,
		ID:          s.ID,
		Ref:         s.Ref,
		Title:       s.Title,
		Description: s.Description,
		Type:        s.Type,
		Pattern:     s.Pattern,
		Format:      s.Format,
		Default:     s.Default,
		Const:       s.Const,
	}

	// 克隆属性
	if s.Properties != nil {
		clone.Properties = make(map[string]*JSONSchema, len(s.Properties))
		for k, v := range s.Properties {
			clone.Properties[k] = v.Clone()
		}
	}

	// 需要克隆铁
	if s.Required != nil {
		clone.Required = make([]string, len(s.Required))
		copy(clone.Required, s.Required)
	}

	// 克隆项目
	clone.Items = s.Items.Clone()

	// 克隆前缀项目
	if s.PrefixItems != nil {
		clone.PrefixItems = make([]*JSONSchema, len(s.PrefixItems))
		for i, item := range s.PrefixItems {
			clone.PrefixItems[i] = item.Clone()
		}
	}

	// 克隆包含
	clone.Contains = s.Contains.Clone()

	// 克隆铁
	if s.Enum != nil {
		clone.Enum = make([]any, len(s.Enum))
		copy(clone.Enum, s.Enum)
	}

	// 克隆实例
	if s.Examples != nil {
		clone.Examples = make([]any, len(s.Examples))
		copy(clone.Examples, s.Examples)
	}

	// 克隆数字指针
	if s.MinLength != nil {
		v := *s.MinLength
		clone.MinLength = &v
	}
	if s.MaxLength != nil {
		v := *s.MaxLength
		clone.MaxLength = &v
	}
	if s.Minimum != nil {
		v := *s.Minimum
		clone.Minimum = &v
	}
	if s.Maximum != nil {
		v := *s.Maximum
		clone.Maximum = &v
	}
	if s.ExclusiveMinimum != nil {
		v := *s.ExclusiveMinimum
		clone.ExclusiveMinimum = &v
	}
	if s.ExclusiveMaximum != nil {
		v := *s.ExclusiveMaximum
		clone.ExclusiveMaximum = &v
	}
	if s.MultipleOf != nil {
		v := *s.MultipleOf
		clone.MultipleOf = &v
	}
	if s.MinItems != nil {
		v := *s.MinItems
		clone.MinItems = &v
	}
	if s.MaxItems != nil {
		v := *s.MaxItems
		clone.MaxItems = &v
	}
	if s.UniqueItems != nil {
		v := *s.UniqueItems
		clone.UniqueItems = &v
	}
	if s.MinContains != nil {
		v := *s.MinContains
		clone.MinContains = &v
	}
	if s.MaxContains != nil {
		v := *s.MaxContains
		clone.MaxContains = &v
	}
	if s.MinProperties != nil {
		v := *s.MinProperties
		clone.MinProperties = &v
	}
	if s.MaxProperties != nil {
		v := *s.MaxProperties
		clone.MaxProperties = &v
	}

	// 克隆额外财产
	if s.AdditionalProperties != nil {
		clone.AdditionalProperties = &AdditionalProperties{
			Allowed: s.AdditionalProperties.Allowed,
			Schema:  s.AdditionalProperties.Schema.Clone(),
		}
	}

	// 克隆图案
	if s.PatternProperties != nil {
		clone.PatternProperties = make(map[string]*JSONSchema, len(s.PatternProperties))
		for k, v := range s.PatternProperties {
			clone.PatternProperties[k] = v.Clone()
		}
	}

	// 克隆属性Names
	clone.PropertyNames = s.PropertyNames.Clone()

	// 克隆组成关键词
	if s.AllOf != nil {
		clone.AllOf = make([]*JSONSchema, len(s.AllOf))
		for i, schema := range s.AllOf {
			clone.AllOf[i] = schema.Clone()
		}
	}
	if s.AnyOf != nil {
		clone.AnyOf = make([]*JSONSchema, len(s.AnyOf))
		for i, schema := range s.AnyOf {
			clone.AnyOf[i] = schema.Clone()
		}
	}
	if s.OneOf != nil {
		clone.OneOf = make([]*JSONSchema, len(s.OneOf))
		for i, schema := range s.OneOf {
			clone.OneOf[i] = schema.Clone()
		}
	}
	clone.Not = s.Not.Clone()

	// 克隆条件关键字
	clone.If = s.If.Clone()
	clone.Then = s.Then.Clone()
	clone.Else = s.Else.Clone()

	// 克隆齿轮
	if s.Defs != nil {
		clone.Defs = make(map[string]*JSONSchema, len(s.Defs))
		for k, v := range s.Defs {
			clone.Defs[k] = v.Clone()
		}
	}

	return clone
}

// JSON将计划序列化为JSON.
func (s *JSONSchema) ToJSON() ([]byte, error) {
	return json.Marshal(s)
}

// 给JSON缩进序列化计划为JSON缩入.
func (s *JSONSchema) ToJSONIndent() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// 从JSON 解析出一个计划 从JSON。
func FromJSON(data []byte) (*JSONSchema, error) {
	var schema JSONSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON schema: %w", err)
	}
	return &schema, nil
}

// 如果需要财产,则需要进行检查。
func (s *JSONSchema) IsRequired(name string) bool {
	for _, req := range s.Required {
		if req == name {
			return true
		}
	}
	return false
}

// GetProperty 以名称返回财产计划 。
func (s *JSONSchema) GetProperty(name string) *JSONSchema {
	if s.Properties == nil {
		return nil
	}
	return s.Properties[name]
}

// 是否有财产 。
func (s *JSONSchema) HasProperty(name string) bool {
	if s.Properties == nil {
		return false
	}
	_, ok := s.Properties[name]
	return ok
}
