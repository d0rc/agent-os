package generics

type ResponseField interface {
	GetName() string
	GetStringValue() string
	GetInnerStructs() []ResponseField
}

type StructResponseFieldType struct {
	Name         string
	InnerStructs []ResponseField
}

func (s StructResponseFieldType) GetName() string {
	return s.Name
}

func (s StructResponseFieldType) GetInnerStructs() []ResponseField {
	return s.InnerStructs
}

func (s StructResponseFieldType) GetStringValue() string {
	return ""
}

type IntResponseFieldType struct {
	Name     string
	Content  string
	MinValue int
	MaxValue int
}

func (r *IntResponseFieldType) GetName() string {
	return r.Name
}

func (r *IntResponseFieldType) GetStringValue() string {
	return r.Content
}

func (r *IntResponseFieldType) GetInnerStructs() []ResponseField {
	return nil
}

type StringResponseFieldType struct {
	Name    string
	Content string
}

func (s *StringResponseFieldType) GetName() string {
	return s.Name
}

func (s *StringResponseFieldType) GetStringValue() string {
	return s.Content
}

func (s *StringResponseFieldType) GetInnerStructs() []ResponseField {
	return nil
}

func ResponseStructure(args ...ResponseField) []ResponseField {
	return args
}

func StringResponseField(name, content string) ResponseField {
	return &StringResponseFieldType{Name: name, Content: content}
}

func StructuredResponseField(name string, structure []ResponseField) ResponseField {
	return &StructResponseFieldType{Name: name, InnerStructs: structure}
}

func IntResponseField(name, content string, minV, maxV int) ResponseField {
	return &IntResponseFieldType{
		Name:     name,
		Content:  content,
		MinValue: minV,
		MaxValue: maxV,
	}
}
