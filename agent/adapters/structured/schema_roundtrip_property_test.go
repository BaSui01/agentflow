package structured

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// Feature: agent-framework-2026-enhancements, Property 7: Schema 生成与解析 Round-Trip
// ** 变动情况:要求3.1、3.5、3.6、4.1、4.3**
//
// 对于任何有效的 从 T 生成 JSON Schema, 然后验证
// 一个有效的 JSON 案例 T 反对 Schema 应该通过; 解析 JSON
// 返回 T 类型应得出等值。

// CroundTripSimpleStruct代表了圆通测试的简单结构.
type RoundTripSimpleStruct struct {
	Name    string  `json:"name" jsonschema:"required"`
	Age     int     `json:"age" jsonschema:"required,minimum=0,maximum=150"`
	Active  bool    `json:"active"`
	Balance float64 `json:"balance"`
}

// CroundTripNestedStruct代表有嵌入对象的支架来进行圆通测试.
type RoundTripNestedStruct struct {
	ID      string               `json:"id" jsonschema:"required"`
	Profile RoundTripProfileInfo `json:"profile" jsonschema:"required"`
	Tags    []string             `json:"tags"`
}

// CroundTripProfileInfo是用于测试的巢状结构.
type RoundTripProfileInfo struct {
	FirstName string `json:"first_name" jsonschema:"required"`
	LastName  string `json:"last_name" jsonschema:"required"`
	Email     string `json:"email" jsonschema:"format=email"`
}

// CroundTripArrayStruct 代表有阵列字段来进行圆通测试的构造.
type RoundTripArrayStruct struct {
	Items  []int     `json:"items" jsonschema:"required,minItems=1"`
	Names  []string  `json:"names"`
	Scores []float64 `json:"scores"`
}

// CroundTripEnumStruct代表了有enum字段的支架来进行圆通测试.
type RoundTripEnumStruct struct {
	Status   string `json:"status" jsonschema:"required,enum=active,inactive,pending"`
	Priority string `json:"priority" jsonschema:"enum=low,medium,high"`
}

// CroundTripComplexStruct代表了具有多功能的复杂结构.
type RoundTripComplexStruct struct {
	ID       string             `json:"id" jsonschema:"required"`
	Name     string             `json:"name" jsonschema:"required,minLength=1,maxLength=100"`
	Count    int                `json:"count" jsonschema:"minimum=0"`
	Enabled  bool               `json:"enabled"`
	Score    float64            `json:"score" jsonschema:"minimum=0,maximum=100"`
	Tags     []string           `json:"tags"`
	Metadata map[string]string  `json:"metadata"`
	SubItems []RoundTripSubItem `json:"sub_items"`
}

// CroundTripSub Project是圆通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通通.
type RoundTripSubItem struct {
	Key   string `json:"key" jsonschema:"required"`
	Value string `json:"value" jsonschema:"required"`
}

// 测试Property SchemaRoundTrip SempleStruct测试 简单结构的圆通.
// ** 变动情况:要求3.1、4.1、4.3**
func TestProperty_SchemaRoundTrip_SimpleStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 生成随机有效实例
		instance := RoundTripSimpleStruct{
			Name:    rapid.StringMatching(`[a-zA-Z]{1,50}`).Draw(rt, "name"),
			Age:     rapid.IntRange(0, 150).Draw(rt, "age"),
			Active:  rapid.Bool().Draw(rt, "active"),
			Balance: rapid.Float64Range(-10000, 10000).Draw(rt, "balance"),
		}

		// 第1步:从 Go struct 类型生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")
		require.NotNil(t, schema, "Schema should not be nil")

		// 步骤2:将实例序列化为JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// 步骤3:根据生成的计划验证JSON
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid instance should pass schema validation")

		// 第4步: 将 JSON 回到结构类型
		var parsed RoundTripSimpleStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// 步骤5:核实等同性
		assert.Equal(t, instance.Name, parsed.Name, "Name should be equal")
		assert.Equal(t, instance.Age, parsed.Age, "Age should be equal")
		assert.Equal(t, instance.Active, parsed.Active, "Active should be equal")
		assert.InDelta(t, instance.Balance, parsed.Balance, 0.0001, "Balance should be approximately equal")
	})
}

// 测试Property SchemaRoundTrip NestedStruct测试为巢状花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花序花
// ** 变动情况:要求3.5、4.1、4.3**
func TestProperty_SchemaRoundTrip_NestedStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 用嵌入对象随机生成有效实例
		instance := RoundTripNestedStruct{
			ID: rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Profile: RoundTripProfileInfo{
				FirstName: rapid.StringMatching(`[A-Z][a-z]{2,20}`).Draw(rt, "firstName"),
				LastName:  rapid.StringMatching(`[A-Z][a-z]{2,20}`).Draw(rt, "lastName"),
				Email:     rapid.StringMatching(`[a-z]{3,10}@[a-z]{3,10}\.[a-z]{2,4}`).Draw(rt, "email"),
			},
			Tags: rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 5).Draw(rt, "tags"),
		}

		// 第1步:从 Go struct 类型生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// 步骤2:将实例序列化为JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// 步骤3:根据生成的计划验证JSON
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid nested instance should pass schema validation")

		// 第4步: 将 JSON 回到结构类型
		var parsed RoundTripNestedStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// 步骤5:核实等同性
		assert.Equal(t, instance.ID, parsed.ID, "ID should be equal")
		assert.Equal(t, instance.Profile.FirstName, parsed.Profile.FirstName, "FirstName should be equal")
		assert.Equal(t, instance.Profile.LastName, parsed.Profile.LastName, "LastName should be equal")
		assert.Equal(t, instance.Profile.Email, parsed.Profile.Email, "Email should be equal")
		assert.Equal(t, instance.Tags, parsed.Tags, "Tags should be equal")
	})
}

// TestProperty SchemaRoundTrip ArrayStruct 测试带阵列的构造的圆通.
// ** 变动情况:要求3.5、4.1、4.3**
func TestProperty_SchemaRoundTrip_ArrayStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 用数组随机生成有效的实例
		instance := RoundTripArrayStruct{
			Items:  rapid.SliceOfN(rapid.IntRange(-1000, 1000), 1, 10).Draw(rt, "items"),
			Names:  rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,15}`), 0, 5).Draw(rt, "names"),
			Scores: rapid.SliceOfN(rapid.Float64Range(0, 100), 0, 5).Draw(rt, "scores"),
		}

		// 第1步:从 Go struct 类型生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// 步骤2:将实例序列化为JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// 步骤3:根据生成的计划验证JSON
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid array instance should pass schema validation")

		// 第4步: 将 JSON 回到结构类型
		var parsed RoundTripArrayStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// 步骤5:核实等同性
		assert.Equal(t, instance.Items, parsed.Items, "Items should be equal")
		assert.Equal(t, instance.Names, parsed.Names, "Names should be equal")
		require.Equal(t, len(instance.Scores), len(parsed.Scores), "Scores length should be equal")
		for i := range instance.Scores {
			assert.InDelta(t, instance.Scores[i], parsed.Scores[i], 0.0001, "Score[%d] should be approximately equal", i)
		}
	})
}

// TestProperty SchemaRoundTrip EnumStruct测试用铝制结构的圆通.
// ** 变动情况:要求3.5、4.1、4.3**
func TestProperty_SchemaRoundTrip_EnumStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 生成带有 enum 值的随机有效实例
		statusValues := []string{"active", "inactive", "pending"}
		priorityValues := []string{"low", "medium", "high"}

		instance := RoundTripEnumStruct{
			Status:   statusValues[rapid.IntRange(0, 2).Draw(rt, "statusIdx")],
			Priority: priorityValues[rapid.IntRange(0, 2).Draw(rt, "priorityIdx")],
		}

		// 第1步:从 Go struct 类型生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// 步骤2:将实例序列化为JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// 步骤3:根据生成的计划验证JSON
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid enum instance should pass schema validation")

		// 第4步: 将 JSON 回到结构类型
		var parsed RoundTripEnumStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// 步骤5:核实等同性
		assert.Equal(t, instance.Status, parsed.Status, "Status should be equal")
		assert.Equal(t, instance.Priority, parsed.Priority, "Priority should be equal")
	})
}

// 测试Property SchemaRoundTrip ComplexStruct 为复杂结构进行圆通测试.
// ** 变动情况:要求3.1、3.5、3.6、4.1、4.3**
func TestProperty_SchemaRoundTrip_ComplexStruct(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 生成随机有效的复杂实例
		numSubItems := rapid.IntRange(0, 3).Draw(rt, "numSubItems")
		subItems := make([]RoundTripSubItem, numSubItems)
		for i := range subItems {
			subItems[i] = RoundTripSubItem{
				Key:   rapid.StringMatching(`[a-z]{3,10}`).Draw(rt, "subKey"),
				Value: rapid.StringMatching(`[a-zA-Z0-9]{5,20}`).Draw(rt, "subValue"),
			}
		}

		numMetadata := rapid.IntRange(0, 3).Draw(rt, "numMetadata")
		metadata := make(map[string]string, numMetadata)
		for i := 0; i < numMetadata; i++ {
			key := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "metaKey")
			value := rapid.StringMatching(`[a-zA-Z0-9]{5,15}`).Draw(rt, "metaValue")
			metadata[key] = value
		}

		instance := RoundTripComplexStruct{
			ID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Name:     rapid.StringMatching(`[a-zA-Z]{1,50}`).Draw(rt, "name"),
			Count:    rapid.IntRange(0, 1000).Draw(rt, "count"),
			Enabled:  rapid.Bool().Draw(rt, "enabled"),
			Score:    rapid.Float64Range(0, 100).Draw(rt, "score"),
			Tags:     rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 5).Draw(rt, "tags"),
			Metadata: metadata,
			SubItems: subItems,
		}

		// 第1步:从 Go struct 类型生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err, "Schema generation should succeed")

		// 步骤2:将实例序列化为JSON
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err, "JSON marshaling should succeed")

		// 步骤3:根据生成的计划验证JSON
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid complex instance should pass schema validation")

		// 第4步: 将 JSON 回到结构类型
		var parsed RoundTripComplexStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err, "JSON unmarshaling should succeed")

		// 步骤5:核实等同性
		assert.Equal(t, instance.ID, parsed.ID, "ID should be equal")
		assert.Equal(t, instance.Name, parsed.Name, "Name should be equal")
		assert.Equal(t, instance.Count, parsed.Count, "Count should be equal")
		assert.Equal(t, instance.Enabled, parsed.Enabled, "Enabled should be equal")
		assert.InDelta(t, instance.Score, parsed.Score, 0.0001, "Score should be approximately equal")
		assert.Equal(t, instance.Tags, parsed.Tags, "Tags should be equal")
		assert.Equal(t, instance.Metadata, parsed.Metadata, "Metadata should be equal")
		assert.Equal(t, len(instance.SubItems), len(parsed.SubItems), "SubItems length should be equal")
		for i := range instance.SubItems {
			assert.Equal(t, instance.SubItems[i].Key, parsed.SubItems[i].Key, "SubItem[%d].Key should be equal", i)
			assert.Equal(t, instance.SubItems[i].Value, parsed.SubItems[i].Value, "SubItem[%d].Value should be equal", i)
		}
	})
}

// 测试Property SchemaRoundTrip 必需的字段Validation测试 执行需要字段.
// ** 参数:要求3.6**
func TestProperty_SchemaRoundTrip_RequiredFieldsValidation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 从需要字段的回合生成计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err, "Schema generation should succeed")

		// 校验计划需要字段
		require.Contains(t, schema.Required, "name", "Schema should mark 'name' as required")
		require.Contains(t, schema.Required, "age", "Schema should mark 'age' as required")

		// 测试缺少所需字段的验证失败
		incompleteData := map[string]any{
			"active":  true,
			"balance": 100.0,
			// 缺少"姓名"和"年龄"
		}
		jsonData, _ := json.Marshal(incompleteData)

		err = validator.Validate(jsonData, schema)
		require.Error(t, err, "Missing required fields should fail validation")

		validationErr, ok := err.(*ValidationErrors)
		require.True(t, ok, "Error should be ValidationErrors type")
		require.NotEmpty(t, validationErr.Errors, "Should have validation errors")
	})
}

// 测试Property SchemaRoundTrip SchemaGeneration Constitutions schema 生成方法一致.
// ** 参数:要求4.3**
func TestProperty_SchemaRoundTrip_SchemaGenerationConsistency(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator1 := NewSchemaGenerator()
		generator2 := NewSchemaGenerator()

		// 从同一种类型生成两次计划
		schema1, err := generator1.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err)

		schema2, err := generator2.GenerateSchema(reflect.TypeOf(RoundTripSimpleStruct{}))
		require.NoError(t, err)

		// Schemas 应该等同
		json1, _ := json.Marshal(schema1)
		json2, _ := json.Marshal(schema2)
		assert.JSONEq(t, string(json1), string(json2), "Schema generation should be consistent")
	})
}

// TestProperty SchemaRoundTrip NilAndEmptyValues 测试零值和空值的处理.
// ** 参数:要求3.5、4.1**
func TestProperty_SchemaRoundTrip_NilAndEmptyValues(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 带有可选字段的测试结构( 没有需要标记)
		type OptionalStruct struct {
			Name  string   `json:"name"`
			Items []string `json:"items"`
		}

		instance := OptionalStruct{
			Name:  rapid.StringMatching(`[a-z]{0,10}`).Draw(rt, "name"),
			Items: rapid.SliceOfN(rapid.StringMatching(`[a-z]{3,10}`), 0, 3).Draw(rt, "items"),
		}

		// 步骤1:产生计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// 第2步:将JSON序列化
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// 步骤3:验证
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Optional fields should pass validation")

		// 第4步:向后分析
		var parsed OptionalStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// 步骤5:核实等同性
		assert.Equal(t, instance.Name, parsed.Name)
		assert.Equal(t, instance.Items, parsed.Items)
	})
}

// TestProperty SchemaRoundTrip NumericControls 测试数字约束验证.
// ** 变动情况:要求3.1、3.5**
func TestProperty_SchemaRoundTrip_NumericConstraints(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 有数值限制的测试结构
		type ConstrainedStruct struct {
			Score int     `json:"score" jsonschema:"required,minimum=0,maximum=100"`
			Rate  float64 `json:"rate" jsonschema:"minimum=0,maximum=1"`
		}

		// 在限制范围内生成有效实例
		instance := ConstrainedStruct{
			Score: rapid.IntRange(0, 100).Draw(rt, "score"),
			Rate:  rapid.Float64Range(0, 1).Draw(rt, "rate"),
		}

		// 步骤1:产生计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// 校验限制在计划之中
		scoreSchema := schema.Properties["score"]
		require.NotNil(t, scoreSchema)
		require.NotNil(t, scoreSchema.Minimum)
		require.NotNil(t, scoreSchema.Maximum)
		assert.Equal(t, float64(0), *scoreSchema.Minimum)
		assert.Equal(t, float64(100), *scoreSchema.Maximum)

		// 第2步:将JSON序列化
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// 步骤3:验证
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid constrained instance should pass validation")

		// 第4步:向后分析
		var parsed ConstrainedStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// 步骤5:核实等同性
		assert.Equal(t, instance.Score, parsed.Score)
		assert.InDelta(t, instance.Rate, parsed.Rate, 0.0001)
	})
}

// TestProperty SchemaRoundTrip Map Fields 为带有地图字段的构造进行圆形测试.
// ** 变动情况:要求3.5、4.1、4.3**
func TestProperty_SchemaRoundTrip_MapFields(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		generator := NewSchemaGenerator()
		validator := NewValidator()

		// 带有地图字段的测试结构
		type MapStruct struct {
			ID       string            `json:"id" jsonschema:"required"`
			Settings map[string]string `json:"settings"`
		}

		// 生成随机映射
		numSettings := rapid.IntRange(0, 5).Draw(rt, "numSettings")
		settings := make(map[string]string, numSettings)
		for i := 0; i < numSettings; i++ {
			key := rapid.StringMatching(`[a-z]{3,8}`).Draw(rt, "settingKey")
			value := rapid.StringMatching(`[a-zA-Z0-9]{5,15}`).Draw(rt, "settingValue")
			settings[key] = value
		}

		instance := MapStruct{
			ID:       rapid.StringMatching(`[a-z0-9]{8,16}`).Draw(rt, "id"),
			Settings: settings,
		}

		// 步骤1:产生计划
		schema, err := generator.GenerateSchema(reflect.TypeOf(instance))
		require.NoError(t, err)

		// 第2步:将JSON序列化
		jsonData, err := json.Marshal(instance)
		require.NoError(t, err)

		// 步骤3:验证
		err = validator.Validate(jsonData, schema)
		require.NoError(t, err, "Valid map instance should pass validation")

		// 第4步:向后分析
		var parsed MapStruct
		err = json.Unmarshal(jsonData, &parsed)
		require.NoError(t, err)

		// 步骤5:核实等同性
		assert.Equal(t, instance.ID, parsed.ID)
		assert.Equal(t, instance.Settings, parsed.Settings)
	})
}
