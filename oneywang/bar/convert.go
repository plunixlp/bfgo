package bar

import (
	"log"
	"math"
	"strconv"
	"strings"
)
import . "github.com/sunwangme/bfgo/api/bfgateway"
import . "github.com/sunwangme/bfgo/api/bfdatafeed"

// 支持这些周期的bar计算
var PeriodKeyList = []BfBarPeriod{
	BfBarPeriod_PERIOD_M01,
	BfBarPeriod_PERIOD_M03,
	BfBarPeriod_PERIOD_M15,
	BfBarPeriod_PERIOD_H01,
	BfBarPeriod_PERIOD_D01}

var periodMinutesList = map[BfBarPeriod]int32{
	BfBarPeriod_PERIOD_M01: 1,
	BfBarPeriod_PERIOD_M03: 3,
	BfBarPeriod_PERIOD_M15: 15}

// "%H:%M:%S.%f"==>"%H:%M:%S"
func ticktime2Bartime(t string, period BfBarPeriod) string {
	var b string

	if dot := strings.LastIndex(t, "."); dot >= 0 {
		b = t[:dot]
		//TODO: 不同period的bar，其bartime应与Period匹配!
	} else {
		log.Fatalf("Failed ticktime2bartime : %s", t)
	}
	return b
}

// 输入："%H:%M:%S"
// 输出：M值
func _bartime2Minute(t string) int32 {
	var m int32
	if strings.Count(t, ":") != 2 {
		log.Fatalf("Failed bartime2minute : %s", t)
	}
	start := strings.Index(t, ":")
	stop := strings.LastIndex(t, ":")
	if stop > start {
		i, err := strconv.Atoi(t[start+1 : stop])
		if err != nil {
			log.Fatalf("Failed bartime2minute : %s, %v", t, err)
		} else {
			m = int32(i)
		}
	}
	return m
}

// 输入："%H:%M:%S"，
// 输出：M值每个周期的整分钟值
func bartime2Minute(t string, period BfBarPeriod) int32 {
	if x, ok := periodMinutesList[period]; ok {
		return _bartime2Minute(t) / x * x
	} else {
		panic("Bartime2Minute: period not supported.")
	}
}

// 输入："%H:%M:%S"
// 输出：H值
func bartime2Hour(t string) int32 {
	var h int32
	if strings.Count(t, ":") != 2 {
		log.Fatalf("Failed bartime2minute : %s", t)
	}
	start := strings.Index(t, ":")
	if start > 0 {
		i, err := strconv.Atoi(t[:start])
		if err != nil {
			log.Fatalf("Failed bartime2minute : %s, %v", t, err)
		} else {
			h = int32(i)
		}
	}
	return h
}

// 输入：两个时间值（不包含日期）与周期
// 输出：是否属于同一个周期
func isSamePeriodTime(previous string, current string, period BfBarPeriod) bool {
	// 只支持分钟与小时，日要用日期而不是时间
	//log.Printf("IsNewPeriod:%v, %v", previous, current)
	if period == BfBarPeriod_PERIOD_H01 {
		return bartime2Hour(previous) == bartime2Hour(current)
	} else {
		// 多分钟的
		t1 := bartime2Minute(previous, period)
		t2 := bartime2Minute(current, period)
		return t1 == t2
	}
	panic("unknow period")
}

// 用Tick数据构造一个新Bar并返回
func constructBarFromTick(t *BfTickData, period BfBarPeriod) *BfBarData {
	b := &BfBarData{Period: period}
	b.Symbol = t.Symbol
	b.Exchange = t.Exchange

	b.ActionDate = t.ActionDate
	b.BarTime = ticktime2Bartime(t.TickTime, period) //"%H:%M:%S.%f"==>"%H:%M:%S"
	b.Volume = t.Volume
	b.OpenInterest = t.OpenInterest
	b.LastVolume = t.LastVolume

	b.OpenPrice = t.LastPrice
	b.HighPrice = t.LastPrice
	b.LowPrice = t.LastPrice
	b.ClosePrice = t.LastPrice

	return b
}

// 用Tick数据更新一个已有Bar
func updateBarFromTick(b *BfBarData, t *BfTickData) {
	b.BarTime = ticktime2Bartime(t.TickTime, b.Period) //"%H:%M:%S.%f"==>"%H:%M:%S"

	b.HighPrice = math.Max(b.HighPrice, t.LastPrice)
	b.LowPrice = math.Min(b.LowPrice, t.LastPrice)
	b.ClosePrice = t.LastPrice

	b.Volume = t.Volume
	b.OpenInterest = t.OpenInterest
	b.LastVolume += t.LastVolume
}

// 保存bar所用的核心数据结构
type BarSlice map[BfBarPeriod]*BfBarData
type Converter struct {
	// 不同品种当前的1分钟K线
	data map[string]*BarSlice
}

func NewConverter() *Converter {
	r := &Converter{data: make(map[string]*BarSlice)}
	return r
}

// 用tick得到某周期的bar
// 返回值
// bool：是否新周期开始
// *BfBarData：如果新周期开始，返回的是上一周期的bar；否则是更新后的老bar
func (p *Converter) Tick2Bar(tick *BfTickData, period BfBarPeriod) (*BfBarData, bool) {
	var ret *BfBarData = nil
	isNewPeriod := false

	id := tick.Symbol + "@" + tick.Exchange
	d, ok := p.data[id]
	if !ok {
		// 这个品种第一次赋值&1分钟的第一次赋值，生成barSlice
		var bs BarSlice = make(map[BfBarPeriod]*BfBarData)
		p.data[id] = &bs
		d = &bs
	}

	if storedBar, ok := (*d)[period]; !ok {
		// 这个周期的bar第一次赋值
		ret = constructBarFromTick(tick, period)
		(*d)[period] = ret
	} else {
		// 判断是否新的周期
		isSamePeriod := true
		if period == BfBarPeriod_PERIOD_D01 {
			isSamePeriod = storedBar.ActionDate == tick.ActionDate
		} else if period == BfBarPeriod_PERIOD_W01 {
			panic("TODO: WEEK BAR not support")
		} else {
			isSamePeriod = isSamePeriodTime(storedBar.BarTime, ticktime2Bartime(tick.TickTime, period), period)
		}

		if isSamePeriod {
			// 还在同一个周期中，更新即可
			updateBarFromTick(storedBar, tick)
		} else {
			// 新的周期开始，需要返回这个完整bar以便插入db，同时生成新周期的bar
			isNewPeriod = true
			// 用tick初始化一个新的currentBar
			(*d)[period] = constructBarFromTick(tick, period)
		}
		ret = storedBar
	}

	return ret, isNewPeriod
}
