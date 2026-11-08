package research

import "testing"

func TestStockCaption(t *testing.T) {
	generic := []string{
		"Picture published with the report of the sentence; its caption only repeats the headline and it is not confirmed to show the defendant, the workshop or the seized hashish.",
		"The page's own picture, captioned only 'Latest Tamil News' and carrying no credit; it is not established to show this seizure.",
		"Page picture captioned only 'Charas smuggler sentenced'; its address is dated 2021 and it is not established to show this case.",
		"File photo of packets of seized hashish oil published with the report.",
		"Picture labelled by the outlet as an illustrative photo.",
		"Abingdon Road in Oxford, shown in a stock photograph.",
		"Archive picture used to illustrate the remand of the man charged in this case.",
		"Bundles of cannabis resin, captioned as an illustration of the drug.",
		"The page's main picture; it does not show the seizure.",
	}
	for _, c := range generic {
		if !StockCaption(c) {
			t.Errorf("not caught: %q", c)
		}
	}
	event := []string{
		"Police officers beside the opened container, with sacks of seized cannabis and paint cans stacked inside. Photo: Dainik Jagran",
		"Men stand behind stacks of green and white sacks at the night-time seizure; the page captioned the picture only with the headline and gave no photo credit.",
		"Police photos of the 52 plants and the grow room in the attic. Photo: Police of the Czech Republic",
		"Police serve a search warrant at the Grouse Grove Avenue grow house on 20 February 2015",
		"Custody photograph of the man arrested, published under his name. Photo: PNC, via Diario El Mundo",
		"Profile photo of the accused released by police.",
	}
	for _, c := range event {
		if StockCaption(c) {
			t.Errorf("event photo dropped: %q", c)
		}
	}
}

func TestDropStockMedia(t *testing.T) {
	kept, dropped := DropStockMedia([]Media{
		{Kind: "photo", Caption: "Seized cannabis plants at the farm. Photo: Police"},
		{Kind: "photo", Caption: "File photo of a police car."},
	})
	if len(kept) != 1 || kept[0].Caption != "Seized cannabis plants at the farm. Photo: Police" || len(dropped) != 1 {
		t.Fatalf("kept %v, dropped %v", kept, dropped)
	}
}
