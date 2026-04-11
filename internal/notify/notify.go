package notify

type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
)

type Event struct {
	Title   string
	Message string
	Level   Level
}

type Sender struct{}

func New() *Sender {
	return &Sender{}
}

func (s *Sender) Send(event Event) error {
	return send(event)
}
