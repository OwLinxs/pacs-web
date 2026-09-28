package studies

import "testing"

func TestQueryV2Validation(t *testing.T) {
	for _, q := range []Query{
		{Limit: 25}, {Limit: 50, Offset: 10000, Sort: "dateAsc", Modality: "CUSTOM_CODE"},
		{Limit: 1, Sort: "dateDesc", Modality: "CT", DateFrom: "2024-02-29", DateTo: "2024-02-29"},
		{Limit: 25, Sort: "native", DateTo: "2026-09-28"}, {Limit: 25, DateFrom: "2026-09-28"},
	} {
		if err := q.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []Query{
		{Limit: 0}, {Limit: 51}, {Limit: 25, Offset: -1}, {Limit: 25, Offset: 10001},
		{Limit: 25, Sort: "arbitrary"}, {Limit: 25, Modality: "CT*"}, {Limit: 25, Modality: "CT\\MR"}, {Limit: 25, Modality: "ct"},
		{Limit: 25, DateFrom: "2026-02-29"}, {Limit: 25, DateFrom: "2026-09-29", DateTo: "2026-09-28"},
	} {
		if q.Validate() == nil {
			t.Fatal("consulta inválida aceita")
		}
	}
}
