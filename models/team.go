package models

type Team struct {
	ID     string `bson:"_id"`
	Name   string `bson:"name"`
	Domain string `bson:"domain"`
	Token  string `bson:"token"`

	Plan string                 `bson:"plan"`
	Icon map[string]interface{} `bson:"icon"`
}
