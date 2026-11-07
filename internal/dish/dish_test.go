package dish

import (
	"strings"
	"testing"
)

func valid() Dish {
	return Dish{
		Name:        "Lembas",
		Description: "Elvish waybread.",
		Price:       Money{Amount: 4550, Currency: "ZAR"},
		Image:       "https://example.com/lembas.jpg",
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*Dish)
		wantErr string // empty: valid
	}{
		{"valid", func(*Dish) {}, ""},
		{"name at the limit", func(d *Dish) { d.Name = strings.Repeat("a", MaxNameLength) }, ""},
		{"name limit counts characters, not bytes", func(d *Dish) { d.Name = strings.Repeat("é", MaxNameLength) }, ""},
		{"missing name", func(d *Dish) { d.Name = "" }, "name is required"},
		{"blank name", func(d *Dish) { d.Name = " \t" }, "name is required"},
		{"long name", func(d *Dish) { d.Name = strings.Repeat("a", MaxNameLength+1) }, "name must be at most 200 characters"},
		{"missing description", func(d *Dish) { d.Description = "" }, "description is required"},
		{"long description", func(d *Dish) { d.Description = strings.Repeat("a", MaxDescriptionLength+1) }, "at most 2000"},
		{"zero amount", func(d *Dish) { d.Price.Amount = 0 }, "price.amount must be a positive"},
		{"negative amount", func(d *Dish) { d.Price.Amount = -1 }, "price.amount must be a positive"},
		{"missing currency", func(d *Dish) { d.Price.Currency = "" }, "price.currency is required"},
		{"other currency", func(d *Dish) { d.Price.Currency = "GBP" }, "price.currency must be ZAR"},
		{"missing image", func(d *Dish) { d.Image = "" }, "image is required"},
		{"long image", func(d *Dish) { d.Image = strings.Repeat("a", MaxImageLength+1) }, "at most 2048"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := valid()
			tt.change(&d)
			err := Validate(d, "ZAR")
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("Validate = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Errorf("Validate = %v, want an error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateUsesTheRestaurantsCurrency(t *testing.T) {
	d := valid()
	d.Price.Currency = "GBP"
	if err := Validate(d, "GBP"); err != nil {
		t.Errorf("GBP dish at a GBP restaurant: %v, want nil", err)
	}
}
