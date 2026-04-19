package agents

type Specialist struct {
	Name           string
	Resume         string
	ExampleRequest string
	// Plugins
}

func ListSpecialists() []Specialist {
	return []Specialist{
		{
			Name:           "Weather Man",
			Resume:         "Hi I'm Weather Man! I have a whole bunch of weather sensors, so I can tell you what the weather is anywhere in the world.",
			ExampleRequest: "Identify the weather in boston",
		},
		{
			Name:           "User Expect",
			Resume:         "Hello I'm User Expert! I keep information about the user, their likes, dislikes, name, age, etc.",
			ExampleRequest: "Clarify user's diet",
		},
		{
			Name:           "Wikipedia",
			Resume:         "Greetings. I am Wikipedia. I hold various trivia and facts about pretty much everything.",
			ExampleRequest: "Find the 61st US president.",
		},
	}
}
