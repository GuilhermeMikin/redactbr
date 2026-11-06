package main

// The identifiers below are generated, like every fixture in this repository; none was collected. They are
// the only personal-looking data the harness handles, and they never reach a model: each text goes through a
// session first, and run stops if a value is left in what would be sent.
const (
	cpfAna    = "123.456.789-09"
	cpfBruno  = "987.654.321-00"
	cpfCarla  = "314.159.265-90"
	cnpjDelta = "11.223.344/5566-13"
	cnpjBeta  = "AB.12C.D34/EF56-02"
	mailAna   = "ana.silva@example.com"
	mailBruno = "bruno.costa@example.org"
)

// systemInstruction is what the README suggests telling a model. Every scenario runs with and without it.
const systemInstruction = "The text contains placeholders for personal data, such as <CPF_k7q2xm9pb3>. Treat them " +
	"as opaque values: whenever you refer to that data, copy the placeholder exactly, character by character, " +
	"including the angle brackets. Do not change, translate, escape or explain placeholders."

type turn struct {
	role string // "user" or "assistant"
	text string
}

// A scenario is a task a pipeline could plausibly send to a model. values are the values the answer should
// mention. A stress scenario asks for something that gives a model a reason to change a token; it is reported
// apart, because it shows how tokens break and not how often they do.
type scenario struct {
	name   string
	stress bool
	turns  []turn
	values []string
}

func user(text string) []turn { return []turn{{"user", text}} }

var scenarios = []scenario{
	{
		name:   "echo",
		turns:  user("Repeat the following line exactly as it is, with nothing before or after it:\nCustomer CPF " + cpfAna + ", email " + mailAna),
		values: []string{cpfAna, mailAna},
	},
	{
		name:   "confirm_pt",
		turns:  user("Olá! Meu CPF é " + cpfAna + " e meu e-mail é " + mailAna + ". Pode confirmar meu cadastro em uma frase curta, citando os dois dados?"),
		values: []string{cpfAna, mailAna},
	},
	{
		name: "summary_pt",
		turns: user("Resuma este chamado em duas frases, citando todos os identificadores:\nA cliente de CPF " + cpfAna + ", e-mail " + mailAna +
			", ligou sobre a nota fiscal do fornecedor CNPJ " + cnpjDelta + ". Ela pediu o reenvio para o mesmo e-mail."),
		values: []string{cpfAna, mailAna, cnpjDelta},
	},
	{
		name: "json_extract",
		turns: user(`Extract the data below into a JSON object with the keys "cpf", "cnpj" and "email". Reply with only the JSON.` +
			"\nCliente: CPF " + cpfBruno + ", empresa CNPJ " + cnpjBeta + ", contato " + mailBruno + "."),
		values: []string{cpfBruno, cnpjBeta, mailBruno},
	},
	{
		name: "table_pt",
		turns: user("Monte uma tabela Markdown com as colunas Nome, CPF e E-mail:\nAna, " + cpfAna + ", " + mailAna +
			"\nBruno, " + cpfBruno + ", " + mailBruno),
		values: []string{cpfAna, mailAna, cpfBruno, mailBruno},
	},
	{
		name:   "translate",
		turns:  user("Translate into English: \"O cliente de CPF " + cpfBruno + " pediu a segunda via do boleto para o e-mail " + mailBruno + ".\""),
		values: []string{cpfBruno, mailBruno},
	},
	{
		name:   "formal_pt",
		turns:  user("Reescreva como um e-mail formal e curto, mantendo os dados: \"oi, sou da empresa de cnpj " + cnpjBeta + ", manda o boleto pro " + mailAna + "\""),
		values: []string{cnpjBeta, mailAna},
	},
	{
		name: "many_values",
		turns: user("Liste em ordem alfabética, um por linha, com o nome e o documento: Carla, CPF " + cpfCarla + "; Bruno, CPF " + cpfBruno +
			"; Ana, CPF " + cpfAna + "; Empresa Delta, CNPJ " + cnpjDelta + "; Empresa Beta, CNPJ " + cnpjBeta +
			"; contato de Ana, " + mailAna + "; contato de Bruno, " + mailBruno + "."),
		values: []string{cpfCarla, cpfBruno, cpfAna, cnpjDelta, cnpjBeta, mailAna, mailBruno},
	},
	{
		name: "history_pt",
		turns: []turn{
			{"user", "Meu CPF é " + cpfCarla + "."},
			{"assistant", "Obrigado, anotei o CPF " + cpfCarla + "."},
			{"user", "Qual é mesmo o meu CPF? Responda só com ele."},
		},
		values: []string{cpfCarla},
	},
	{
		// The answer must pick one token of two: another token in it means a model chose the wrong one.
		name:   "choose_one_pt",
		turns:  user("Ana (CPF " + cpfAna + ") comprou R$ 120. Bruno (CPF " + cpfBruno + ") comprou R$ 90. Quem comprou mais? Responda só com o CPF dessa pessoa."),
		values: []string{cpfAna},
	},
	{
		name:   "html",
		stress: true,
		turns:  user("Write an HTML <ul> with one <li> for each of these contact details: CPF " + cpfAna + ", email " + mailAna + ". Reply with only the HTML."),
		values: []string{cpfAna, mailAna},
	},
	{
		name:   "all_caps",
		stress: true,
		turns:  user("Rewrite this sentence in ALL CAPS: the customer with CPF " + cpfBruno + " wrote from " + mailBruno + "."),
		values: []string{cpfBruno, mailBruno},
	},
}
