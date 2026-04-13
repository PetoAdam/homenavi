package email

// Config is the business-facing email delivery configuration.
type Config struct {
	FromEmail string
	FromName  string
	AppName   string
}

// EmailData is the template payload for rendered emails.
type EmailData struct {
	UserName string
	Code     string
	Subject  string
	Title    string
	Message  string
	AppName  string
}
