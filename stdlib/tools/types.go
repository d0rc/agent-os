package tools

type MapKV struct {
	Key      string
	Value    interface{}
	InnerMap []MapKV
}
