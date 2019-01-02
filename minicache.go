package minicache

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	NoExpiration      time.Duration = -1
	defaultExpiration time.Duration = 0
)

type Item struct {
	Object     interface{}
	Expiration int64
}

type Minicache struct {
	defaultExpiration time.Duration
	items             map[string]Item
	rwmtx             sync.RWMutex
	gcInterval        time.Duration
	stopGc            chan bool
}

func (item Item) IsExpired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

//循环gc
func (minic *Minicache) gcLoop() {
	ticker := time.NewTicker(minic.gcInterval) //初始化定时器
	for {
		select {
		case <-ticker.C:
			minic.DeleteExpired()
		case <-minic.stopGc:
			ticker.Stop()
			return
		}
	}
}

//过期缓存删除
func (minic *Minicache) DeleteExpired() {
	now := time.Now().UnixNano()
	minic.rwmtx.Lock()
	defer minic.rwmtx.Unlock()
	for k, v := range minic.items {
		if v.Expiration > 0 && now > v.Expiration {
			minic.delete(k)
		}
	}
}

//删除
func (minic *Minicache) delete(k string) {
	delete(minic.items, k)
}

//删除操作
func (minic *Minicache) Delete(k string) {
	minic.rwmtx.Lock()
	minic.delete(k)
	defer minic.rwmtx.Unlock()
}

//设置缓存数据项,存在就覆盖
func (minic *Minicache) Set(k string, v interface{}, d time.Duration) {
	var e int64
	if d == defaultExpiration {
		d = minic.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	minic.rwmtx.Lock()
	defer minic.rwmtx.Unlock()
	minic.items[k] = Item{
		Object:     v,
		Expiration: e,
	}
}

//设置数据项,无锁
func (minic *Minicache) set(k string, v interface{}, d time.Duration) {
	var e int64
	if d == defaultExpiration {
		d = minic.defaultExpiration
	}
	if d > 0 {
		e = time.Now().Add(d).UnixNano()
	}
	minic.items[k] = Item{
		Object:     v,
		Expiration: e,
	}
}

//获取数据项,并判断数据项是否过期
func (minic *Minicache) get(k string) (interface{}, bool) {
	item, found := minic.items[k]
	if !found || item.IsExpired() {
		return nil, false
	}
	return item.Object, true
}

//新增操作,如果数据项存在,则报错
func (minic *Minicache) Add(k string, v interface{}, d time.Duration) error {
	minic.rwmtx.Lock()
	_, found := minic.get(k)
	if found {
		minic.rwmtx.Unlock()
		return fmt.Errorf("Item %s already exists", k)
	}
	minic.set(k, v, d)
	minic.rwmtx.Unlock()
	return nil
}

//获取缓存操作
func (minic *Minicache) Get(k string) (interface{}, bool) {
	minic.rwmtx.RLock()
	item, found := minic.items[k]
	if !found || item.IsExpired() {
		minic.rwmtx.RUnlock()
		return nil, false
	}
	minic.rwmtx.RUnlock()
	return item.Object, true
}

//替换缓存
func (minic *Minicache) Replace(k string, v interface{}, d time.Duration) error {
	minic.rwmtx.Lock()
	_, found := minic.get(k)
	if !found {
		minic.rwmtx.Unlock()
		return fmt.Errorf("Item %s does not exists", k)
	}
	minic.set(k, v, d)
	minic.rwmtx.Unlock()
	return nil
}

//缓存数据写入io.Writer中
func (minic *Minicache) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error registering item types with gob library")
		}
	}()
	minic.rwmtx.Lock()
	defer minic.rwmtx.Unlock()
	for _, v := range minic.items {
		gob.Register(v.Object)
	}
	err = enc.Encode(&minic.items)
	return
}

//序列化到文件
func (minic *Minicache) SaveToFile(fileName string) error {
	f, err := os.Create(fileName)
	if err != nil {
		return err
	}
	if err = minic.Save(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

//从io.Reader读取
func (minic *Minicache) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := make(map[string]Item, 0)
	err := dec.Decode(&items)
	if err != nil {
		return err
	}
	minic.rwmtx.Lock()
	defer minic.rwmtx.Unlock()
	for k, v := range items {
		obj, ok := minic.items[k]
		if !ok || obj.IsExpired() {
			minic.items[k] = v
		}
	}
	return nil
}

//从文件中读取
func (minic *Minicache) LoadFromFile(fileName string) error {
	f, err := os.Open(fileName)
	if err != nil {
		return err
	}
	if err = minic.Load(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

//返回缓存中数据项数量
func (minic *Minicache) Count() int {
	minic.rwmtx.Lock()
	defer minic.rwmtx.Unlock()
	return len(minic.items)
}

//清空缓存
func (minic *Minicache) Flush() {
	minic.rwmtx.RLock()
	defer minic.rwmtx.RUnlock()
	minic.items = map[string]Item{}
}

//停止gc
func (minic *Minicache) Stopgc() {
	minic.stopGc <- true
}

//创建缓存
func NewMiniCache(defaultExpiration, gcInterval time.Duration) (minic *Minicache) {
	minic = &Minicache{
		defaultExpiration: defaultExpiration,
		gcInterval:        gcInterval,
		items:             map[string]Item{},
		stopGc:            make(chan bool),
	}
	go minic.gcLoop()
	return
}
