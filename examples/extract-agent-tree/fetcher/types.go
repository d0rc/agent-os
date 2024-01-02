package fetcher

type DbMessage struct {
	Id      string  `db:"id"`
	Name    *string `db:"name"`
	Role    string  `db:"role"`
	Content string  `db:"content"`
}

type DbMessageLink struct {
	Id      string `db:"id"`
	ReplyTo string `db:"reply_to"`
}

type DotNode struct {
	Id      string
	Label   string
	Role    string
	Content string
}
