package message

import "github.com/tychoish/grip/level"

type stringMessage struct {
	Message string `bson:"message" json:"message" yaml:"message"`
	Base    `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// NewDefaultMessage provides a Composer interface around a single
// string, which are always logable unless the string is empty.
func NewDefaultMessage(p level.Priority, message string) Composer {
	m := &stringMessage{
		Message: message,
	}

	m.SetPriority(p)

	return m
}

func NewString(m string) Composer {
	return &stringMessage{Message: m}
}

func (s *stringMessage) Resolve() string {
	return s.Message
}

func (s *stringMessage) Loggable() bool {
	return s.Message != ""
}

func (s *stringMessage) Raw() interface{} {
	_ = s.Collect()
	return s
}
