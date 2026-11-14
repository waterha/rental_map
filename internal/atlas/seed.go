package atlas

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func Seed(ctx context.Context, s *Store, m *Maps) error {
	var count int
	if err := s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM listings").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	cities := []struct {
		code        string
		base        int
		communities []string
	}{
		{"330100", 2800, []string{"桂语江南", "西溪悦城", "滨江星光里", "运河上城"}},
		{"310000", 4800, []string{"梧桐公寓", "苏河湾", "前滩悦居", "静安新里"}},
		{"440300", 3800, []string{"湾畔花园", "科技园公寓", "塘朗雅苑", "南山悦居"}},
		{"110000", 4200, []string{"望京新城", "西山悦府", "东湖湾", "青年汇"}},
		{"510100", 1800, []string{"麓湖小筑", "锦江春色", "天府新苑", "玉林公寓"}},
		{"440100", 2600, []string{"珠江新苑", "天河雅居", "江南小筑", "越秀时光"}},
		{"320100", 2100, []string{"金陵新寓", "云锦花园", "河西悦府", "梧桐里"}},
		{"420100", 1600, []string{"东湖新城", "光谷青年城", "江岸时光", "武昌小筑"}},
		{"320500", 2200, []string{"湖畔花园", "姑苏雅苑", "金鸡湖公寓", "苏悦新居"}},
		{"610100", 1700, []string{"曲江新苑", "长安小筑", "高新悦居", "大唐雅居"}},
		{"500000", 1500, []string{"山城公寓", "江北悦府", "南滨江苑", "两江新居"}},
		{"350200", 2400, []string{"海沧新苑", "鹭岛小筑", "集美悦居", "海景花园"}},
		{"370200", 1900, []string{"海岸公寓", "浮山新苑", "崂山悦居", "市南花园"}},
		{"430100", 1400, []string{"梅溪新居", "湘江悦府", "岳麓小筑", "星城公寓"}},
		{"410100", 1300, []string{"郑东悦居", "金水新苑", "中原花园", "青年公寓"}},
		{"340100", 1400, []string{"天鹅湖公寓", "滨湖悦居", "蜀山新苑", "包河小筑"}},
		{"530100", 1500, []string{"滇池新苑", "春城小筑", "翠湖雅居", "西山公寓"}},
		{"120000", 1900, []string{"海河新苑", "南开雅居", "津门公寓", "河西悦府"}},
	}
	all := m.List("")
	now := time.Now().UTC()
	for cityIndex, c := range cities {
		city, ok := m.Get(c.code)
		if !ok {
			continue
		}
		districts := []Region{}
		for _, r := range all {
			if r.Parent == c.code && r.Level == "district" {
				districts = append(districts, r)
			}
		}
		if len(districts) == 0 {
			continue
		}
		province := c.code
		if city.Level != "province" {
			province = city.Parent
		}
		for i := 0; i < 12; i++ {
			d := districts[i%len(districts)]
			kind := []string{"整租", "合租", "转租"}[i%3]
			rooms := i%3 + 1
			area := float64(32 + i*7)
			price := c.base + (i%5)*420
			if kind == "合租" {
				price = c.base/2 + (i%4)*180
				area = float64(16 + i%5*4)
				rooms = 1
			}
			titles := []string{"阳光满屋 · 南向通透的理想居所", "地铁旁的温暖小窝，等一个爱生活的你", "工作调动急转 · 精装好房随时入住", "大落地窗，把城市风景装进生活", "独立空间 · 和有趣的人做邻居", "租期灵活 · 住进有烟火气的街区"}
			tags := [][]string{{"近地铁", "朝南", "可养宠物"}, {"独立阳台", "电梯房", "可做饭"}, {"近地铁", "拎包入住", "租期灵活"}, {"朝南", "独立阳台", "电梯房"}}
			name := c.communities[i%len(c.communities)]
			l := Listing{ID: fmt.Sprintf("demo-%02d-%02d", cityIndex, i), Title: titles[i%len(titles)], Description: "这是一条用于体验租房版图的示例房源，不代表真实出租信息。房源布局舒适，采光明亮，周边生活配套便利。你可以试用地图筛选、收藏及发布功能，探索适合自己的城市与居住方式。", Type: kind, Price: price, Area: area, Rooms: rooms, Floor: fmt.Sprintf("%d / %d 层", i+3, 18+i%3*7), Orientation: []string{"朝南", "南北通透", "东南"}[i%3], Community: name, ProvinceCode: province, CityCode: c.code, DistrictCode: d.Code, City: city.Name, District: d.Name, Address: city.Name + d.Name + name, Longitude: d.Center[0] + float64(i%3)*0.005, Latitude: d.Center[1] + float64(i%4)*0.004, Image: fmt.Sprintf("/images/home-%d.jpg", i%8+1), Tags: tags[i%len(tags)], Contact: "示例房源，无真实联系方式", CreatedAt: now.Add(-time.Duration(cityIndex*13+i) * time.Hour).Format(time.RFC3339), IsDemo: true, Owner: "demo"}
			if err := s.Insert(ctx, l); err != nil {
				return err
			}
		}
	}
	return nil
}

func containsFold(value, query string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(query))
}
