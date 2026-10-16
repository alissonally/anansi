# Etapa 7 — Sessões

**Status:** concluída.

Backend em `internal/sshtunnel` (túnel e verificação de chaves), `internal/connections` (perfil, segredos,
sessões, exportação), `internal/database/mysql` (socket, discagem pelo túnel, script inicial) e
`internal/api/sessions.go`. Frontend: aba **Rede / SSH** e campos novos em **Avançado** no diálogo de conexão,
fluxo de conexão em `stores/connections.ts` e pastas na árvore.

## Túnel SSH (HeidiSQL: "SSH tunnel" via plink)

- Cliente SSH embutido (`golang.org/x/crypto/ssh`); **nenhuma porta local é aberta**: o driver do MySQL disca
  cada conexão do pool por um canal `direct-tcpip` do mesmo cliente SSH. Host e porta da aba Geral são vistos a
  partir do servidor SSH. **Melhoria**: o HeidiSQL depende do plink/ssh externo e de uma porta local.
- Autenticação por **senha** (também responde a keyboard-interactive), **chave privada** (OpenSSH/PEM, com ou
  sem frase secreta) ou **agente SSH** (`SSH_AUTH_SOCK`).
- A senha/frase secreta é um segredo como a senha do banco: memória da sessão ou keyring (`ssh:<id>`) quando o
  perfil pede; nunca vai para `connections.json` nem para a exportação. Sem segredo, a conexão pede pelo
  diálogo (com "salvar no keyring").
- **Verificação de chave do servidor sempre ligada**: `~/.ssh/known_hosts` (somente leitura) e o
  `known_hosts` do aplicativo. Servidor desconhecido → a conexão para e mostra tipo e impressão digital
  SHA256 para o usuário confirmar; só a chave realmente apresentada pelo servidor (com a mesma impressão
  digital) pode ser gravada. Chave diferente da conhecida → erro sem opção de continuar (possível
  man-in-the-middle).
- Keep-alive do SSH (`keepalive@openssh.com`) no intervalo do perfil (padrão 30 s). O túnel é fechado junto
  com a sessão.
- Os canais SSH não suportam prazos de leitura/escrita, que o driver usa: um adaptador implementa os prazos
  com temporizadores (prazo vencido fecha o canal, que o driver trata como queda da conexão).

## Socket Unix (HeidiSQL: "Named pipe / socket")

Caminho absoluto do socket em vez de host e porta (host passa a ser opcional). Não combina com SSH. TLS sobre
socket usa `localhost` como nome do servidor.

## Script inicial (HeidiSQL: "Startup script")

Instruções separadas por `;` executadas em **cada nova conexão física** do pool (o pool abre conexões sob
demanda, então rodar só uma vez não bastaria). Se uma falhar, a conexão falha com a instrução na mensagem, em
vez de entregar uma sessão pela metade. Validação ao salvar: controle de transação é recusado; em perfis
somente leitura só `SET` e leituras.

## Keep-alive (HeidiSQL: "Ping every X seconds")

Ping periódico no pool (5 a 3600 s; 0 desliga) para que sessões ociosas, NAT e túneis não caiam.

## Organização (HeidiSQL: pastas e comentários no gerenciador de sessões)

- **Pastas** com subpastas (`Clientes/ACME`), mostradas na árvore com contagem, recolhíveis; "Mover para a
  pasta…" e "Renomear pasta…" no menu.
- **Comentário** do perfil (aparece na dica da conexão na árvore).
- **Databases exibidos** (HeidiSQL: "Databases"): nomes ou padrões com `*` e `?` separados por `;`
  limitando o que a árvore lista.
- **Estatísticas** (HeidiSQL: "Statistics"): conexões bem-sucedidas, falhas e data da última conexão,
  mantidas pelo aplicativo (o que vem da interface é ignorado) e mostradas na aba Avançado com a data de
  criação.

## Exportar/importar conexões (HeidiSQL: "Export/Import settings")

Menu da conexão e das pastas: exportar uma, as da pasta ou todas para um arquivo JSON; importar acrescenta
com novos ids e renomeia nomes repetidos ("Nome (2)"); perfis inválidos são ignorados e listados. **Senhas e
segredos SSH nunca são exportados** (o HeidiSQL exporta as senhas, apenas ofuscadas).

## Testes

- `sshtunnel`: servidor SSH de teste em processo (`sshtunnel/sshtest`, senha, chave e encaminhamento
  `direct-tcpip`); host desconhecido com impressão digital, recusa de confiar numa impressão digital que o
  servidor não apresentou, conexão após confiar, senha errada e ausente, **chave trocada recusada**, chave
  ed25519 com frase secreta, prazos do adaptador.
- `connections`: exportação/importação sem segredos (arquivo de configuração e exportação conferidos),
  nomes repetidos, arquivo estranho recusado, segredo SSH removido do keyring ao excluir, validações (socket
  + SSH, usuário/chave SSH, socket relativo, transação e escrita no script inicial), estatísticas (reuso da
  sessão não conta; salvar não altera).
- Integração (MySQL 8.4 e MariaDB 11.4): conexão **pelo túnel SSH** ao servidor real (segredo pedido →
  host desconhecido → confiar → consultas pelo túnel → desconectar/reconectar → testar perfil não salvo com
  senha certa e errada); conexão por **socket Unix** (proxy local) com **script inicial** aplicado em todas
  as conexões do pool e script com erro recusando a conexão.
- No app: perfil novo com SSH, pasta, filtro e script inicial pelo diálogo; teste com confirmação da chave;
  árvore com `Clientes/ACME` e só `demo_shop`; variável do script lida numa consulta.

## Limitações

- Sem saltos encadeados (ProxyJump) nem encaminhamento por porta local para outras ferramentas.
- Named pipes do Windows não são suportados (o driver usa TCP ou socket Unix).
