package subscriptions

import (
	"encoding/json"
	"errors"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/log"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
)

const (
	Folder = "subscriptions"
)

type Manager struct {
	datadir string
	list    []*Subscription
	mutex   sync.Mutex
}

func NewManager(datadir string) (*Manager, error) {
	m := &Manager{
		datadir: datadir,
	}

	file, err := m.openFile()
	defer file.Close()
	if err == nil {
		data, err := ioutil.ReadAll(file)
		if err != nil {
			return nil, err
		}

		list := []*Subscription{}
		if err := json.Unmarshal(data, &list); err != nil {
			log.Warn("cannot parse subscriptions.json", "err", err)
		}
		m.list = list
	}
	return m, nil
}

func (m *Manager) Subscribe(contract common.Address, event string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, s := range m.list {
		if s.Contract == contract && s.Event == event {
			return errors.New("subscription exists")
		}
	}

	m.list = append(m.list, &Subscription{
		Contract: contract,
		Event:    event,
	})
	return m.persist()
}

func (m *Manager) persist() error {
	file, err := m.openFile()
	defer file.Close()
	if err != nil {
		return err
	}
	data, err := json.Marshal(m.list)
	if err != nil {
		return err
	}
	if err := file.Truncate(0); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	return nil
}

func (m *Manager) openFile() (file *os.File, err error) {
	newpath := filepath.Join(m.datadir, Folder)
	if err := os.MkdirAll(newpath, os.ModePerm); err != nil {
		return nil, err
	}
	filePath := filepath.Join(newpath, "subscriptions.json")
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (m *Manager) RawSubscriptions() [][]byte {
	var result [][]byte
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, s := range m.list {
		result = append(result, append(s.Contract.Bytes(), []byte(s.Event)...))
	}
	return result
}

func (m *Manager) Subscriptions() []*Subscription {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	result := make([]*Subscription, len(m.list))
	copy(result, m.list)
	return result
}

func (m *Manager) Unsubscribe(contract common.Address, event string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	idx := -1
	for i, s := range m.list {
		if s.Contract == contract && s.Event == event {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errors.New("subscription is not found")
	}

	copy(m.list[idx:], m.list[idx+1:])
	m.list[len(m.list)-1] = nil
	m.list = m.list[:len(m.list)-1]

	return m.persist()
}

type Subscription struct {
	Contract common.Address `json:"contract"`
	Event    string         `json:"event"`
}
