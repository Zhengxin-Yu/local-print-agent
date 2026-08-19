package config

type Config struct {
	Host      string
	FirstPort int
	LastPort  int
	DataDir   string
}

func Default() Config {
	return Config{
		Host:      "127.0.0.1",
		FirstPort: 17653,
		LastPort:  17660,
		DataDir:   "data",
	}
}

func (c Config) CandidatePorts() []int {
	ports := make([]int, 0, c.LastPort-c.FirstPort+1)
	for port := c.FirstPort; port <= c.LastPort; port++ {
		ports = append(ports, port)
	}
	return ports
}
