package conf

type Config struct {
	ApiKey string `env:"API_KEY,required"`
}
