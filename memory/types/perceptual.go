package types

type PerceptualMemory struct{}

func (p *PerceptualMemory) Add(item MemoryItem) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (p *PerceptualMemory) Retrieve(query string, limit int64, metadata map[string]string) ([]MemoryItem, error) {
	//TODO implement me
	panic("implement me")
}

func (p *PerceptualMemory) Delete(id string) error {
	//TODO implement me
	panic("implement me")
}

func (p *PerceptualMemory) Status() MemoryStatus {
	//TODO implement me
	panic("implement me")
}
