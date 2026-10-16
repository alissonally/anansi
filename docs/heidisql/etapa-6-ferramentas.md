# Etapa 6 — Ferramentas

**Status:** concluída.

Backend em `internal/database/mysql/tools.go` (SQL de manutenção, busca, edição em massa, FLUSH),
`internal/database/mysql/users.go` (contas e privilégios), `internal/api/tools.go` e `internal/api/compare.go`.
Frontend: `TableToolsDialog.vue`, `CopyTableDialog.vue`, `GenerateDataDialog.vue`, `CompareDialog.vue` e a aba
`UserManager.vue` (tipo de aba `users`). As ferramentas ficam no submenu **Ferramentas** da árvore (conexão,
database, tabela) e da aba do database, e em botões e menus da aba do servidor.

## Ferramentas de tabela (HeidiSQL: "Table tools")

Um diálogo com a seleção de databases e tabelas à esquerda e três abas:

- **Manutenção**: `CHECK`, `ANALYZE`, `CHECKSUM`, `OPTIMIZE` e `REPAIR TABLE` com as opções válidas de cada um
  (`QUICK`, `FAST`, `MEDIUM`, `EXTENDED`, `CHANGED`, `FOR UPGRADE`, `USE_FRM`). As opções são validadas no
  backend (CHECKSUM aceita `QUICK` ou `EXTENDED`, não os dois). Roda como tarefa: uma linha por mensagem do
  servidor (tabela, operação, tipo, mensagem), erros por tabela não interrompem as demais. `OPTIMIZE` e
  `REPAIR` alteram tabelas e seguem as políticas de escrita (somente leitura bloqueia, produção confirma).
- **Buscar texto** (HeidiSQL: "Find text on server"): procura em todas as colunas das tabelas e views
  selecionadas. Modos: contém, igual, começa com, termina com e **expressão regular** (melhoria); diferenciar
  maiúsculas; filtrar por tipo de coluna (texto, numéricas, data/hora, binárias). Detalhes:
  - o texto vai como **valor vinculado** e `%`, `_` e `!` são escapados (`LIKE … ESCAPE '!'`): buscar `100%`
    ou `a_b` procura o texto literal;
  - colunas de texto são comparadas com `CONVERT(… USING utf8mb4)` (tabelas em charsets diferentes);
    maiúsculas: `COLLATE utf8mb4_bin` ou `LOWER()` dos dois lados;
  - regex: `REGEXP_LIKE(expr, ?, 'i'|'c')` no MySQL e `REGEXP` com `(?i)`/`(?-i)` no MariaDB (PCRE);
  - colunas numéricas só entram com igualdade e só quando o texto é um número (como no HeidiSQL);
  - um `SELECT COUNT(*)` por tabela, em tarefa cancelável; o resultado aparece conforme chega.
  Com duplo clique num resultado a tabela abre **já filtrada** (o filtro é a condição com os valores como
  literais).
- **Edição em massa** (HeidiSQL: "Bulk table editor"): mover para outro database (`RENAME TABLE`), trocar a
  engine, **converter os dados** para outro charset/collation (`CONVERT TO CHARACTER SET`), mudar só a
  collation padrão e reiniciar o `AUTO_INCREMENT`. Prévia das instruções; cada tabela é tentada mesmo que
  outra falhe.

## Copiar tabela (HeidiSQL: "Create table copy")

Para o mesmo ou outro database, com escolha de colunas, índices (a chave primária sempre vai), chaves
estrangeiras, CHECKs, dados com filtro `WHERE` opcional e "substituir se já existir". A estrutura vem do editor
de tabela do backend (a mesma usada no ALTER), não de um `CREATE TABLE … LIKE`.

- Os nomes de FKs e CHECKs passam a ser `<copia>_ibfk_N`/`<copia>_chk_N`: no MySQL esses nomes são únicos por
  database e a cópia com os nomes originais falharia.
- Copiando para outro database as FKs continuam apontando para as tabelas do database de origem.
- Colunas geradas não entram no `INSERT … SELECT` (são recalculadas); `AUTO_INCREMENT` sem índice é removido
  quando os índices não são copiados (o servidor exige o índice).
- Executa com `FOREIGN_KEY_CHECKS=0` e as políticas de escrita.

## Gerar dados de teste (HeidiSQL: "Generate data")

N linhas aleatórias (até 1.000.000) com porcentagem de NULL nas colunas anuláveis: inteiros dentro da faixa do
tipo (`TINYINT(1)` como 0/1), DECIMAL, datas e horas, ENUM/SET com os valores definidos, JSON, textos de
palavras respeitando o tamanho da coluna e binários. Pula `AUTO_INCREMENT`, colunas geradas e espaciais (e
recusa quando uma coluna obrigatória não pode ser gerada). `INSERT IGNORE` em lotes com valores vinculados
(respeitando o limite de parâmetros) e chaves estrangeiras desligadas, em tarefa cancelável.

## Gerenciador de usuários (HeidiSQL: "User manager")

Aba **Usuários** (menu da conexão, botão na aba do servidor), com lista de contas e roles (bloqueadas e sem
senha sinalizadas) e editor em quatro seções:

- **Credenciais**: usuário, host (com sugestões), senha (em branco = manter; gerador de senha aleatória),
  plugin de autenticação (lista do servidor), bloqueio da conta, expiração da senha (padrão, imediata, nunca,
  a cada N dias).
- **Privilégios**: objetos global, database (aceita curingas), tabela e rotina; os privilégios de cada nível
  vêm de `SHOW PRIVILEGES` (inclui os dinâmicos do MySQL 8); `ALL PRIVILEGES`; `WITH GRANT OPTION`;
  **privilégios por coluna** em tabelas.
- **Roles** (MySQL 8 e MariaDB): roles concedidas, `WITH ADMIN OPTION` e roles padrão.
- **Limites e segurança**: limites por hora e conexões simultâneas; `REQUIRE` NONE/SSL/X509/cipher, issuer,
  subject.

Como funciona:

- A conta é lida de `SHOW CREATE USER` e `SHOW GRANTS` (`ParseGrants` junta as linhas de privilégios dinâmicos
  e ignora `USAGE` e `PROXY`); roles do MySQL vêm de `mysql.role_edges`/`mysql.default_roles`, do MariaDB de
  `mysql.user.is_role`/`default_role`. Nomes de database são mantidos como o servidor mostra (`\_`), para que o
  `REVOKE` atinja o mesmo padrão.
- Salvar **compara com a definição atual no servidor** (não com a cópia do editor) e gera só as diferenças:
  `CREATE USER`/`CREATE ROLE`, `RENAME USER`, `ALTER USER` (senha, plugin, REQUIRE, limites, bloqueio,
  expiração), `GRANT`/`REVOKE` por objeto, roles e `SET DEFAULT ROLE` (sintaxe de cada servidor). Carregar e
  salvar sem mudanças gera zero instruções (teste de ida e volta).
- **Senhas nunca aparecem**: a prévia, a confirmação de produção, o log de SQL e os resultados mostram
  `IDENTIFIED BY '***'`.
- Excluir sempre pede confirmação.
- **Alterar a minha senha** (HeidiSQL: "Change password"): `ALTER USER CURRENT_USER()`, atualiza a senha
  lembrada (memória e keyring, quando o perfil salva a senha) e reconecta.

## FLUSH

Botão **Flush** na aba do servidor: `PRIVILEGES`, `HOSTS`, `TABLES`, `STATUS`, `LOGS`, `USER_RESOURCES` e
`QUERY CACHE` (lista fechada validada no backend; segue as políticas de escrita).

## Comparar estruturas (melhoria; HeidiSQL só tem "Synchronize" de dados via export)

Compara dois databases da mesma ou de conexões diferentes e lista, por objeto (tabelas, views, procedures,
functions, triggers e eventos), **criar** (só na origem), **excluir** (só no destino), **alterar** ou **igual**,
com as instruções para o destino e as duas definições lado a lado.

- Tabelas: a definição de origem é transportada para o destino e o diff do editor de tabela gera `ALTER TABLE`
  (colunas, índices, FKs e CHECKs ligados pelo nome) — dados preservados. `AUTO_INCREMENT` não conta como
  diferença.
- Demais objetos: os dois lados são gerados pelos mesmos construtores (sem `DEFINER`, referências ao database
  de origem trocadas pelo destino) e comparados com espaços normalizados; diferentes viram `DROP … IF EXISTS`
  + `CREATE`.
- Exclusões vêm **desmarcadas**. Aplicar executa no destino com `FOREIGN_KEY_CHECKS=0` (opção "continuar após
  erros") e compara de novo; ou "Abrir script no editor" (com `DELIMITER`).

## Testes

- Unitários (`users_test.go`): parse de `SHOW GRANTS` (MySQL e MariaDB, dinâmicos, colunas, rotinas, roles),
  diff de contas, ida e volta sem instruções, máscara de senhas; `tools.go` (opções de manutenção, escapes da
  busca).
- Integração (`internal/integration/tools_test.go`, MySQL 8.4 e MariaDB 11.4):
  - manutenção CHECK/ANALYZE/CHECKSUM/OPTIMIZE e recusa de opções conflitantes;
  - busca: contém, maiúsculas, `%` e `_` literais, numérica, regex, igualdade; o filtro devolvido conta as
    mesmas linhas no grid;
  - cópia com índices, FK, CHECK, coluna gerada e filtro; para outro database com colunas escolhidas (FK
    aponta para a origem); substituir existente; recusa de copiar sobre si mesma;
  - geração de 1.200 linhas com NULLs; edição em massa (conversão de charset + mover); FLUSH e recusa de
    FLUSH inválido;
  - usuários: role, usuário com senha, limites, REQUIRE SSL, expiração, privilégios global/database (com
    GRANT OPTION)/tabela (com coluna)/rotina, role e role padrão; prévia sem a senha; ida e volta sem
    instruções; renomear + trocar host + bloquear + revogar; excluir usuário e role;
  - comparação: tabela diferente, só na origem, só no destino, view, function diferente, procedure, trigger e
    evento → aplicar → nova comparação toda igual, dados do destino preservados.
- No app (Playwright): ANALYZE em duas tabelas, busca abrindo a tabela filtrada, cópia, geração de 1.000
  linhas, comparação, criação de usuário pela aba (prévia mascarada) e FLUSH PRIVILEGES.

## Correções encontradas no caminho

- `SHOW CREATE USER` devolve uma coluna (o leitor esperava duas).
- Linhas `GRANT USAGE ON *.*` viravam um objeto global vazio na lista de privilégios.
- `DROP ROLE` no MariaDB não aceita `@host`.
- O painel de tarefas aberto automaticamente fechava só à mão e cobria a árvore; agora fecha sozinho quando
  todas as tarefas terminam sem avisos.

## Limitações

- Ferramentas disponíveis para MySQL/MariaDB (as outras engines não implementam as interfaces).
- A comparação é só de estrutura; partições entram pela cláusula inteira. Corpos de rotinas com referências
  qualificadas ao database de origem não são reescritos.
- Ao trocar a própria senha, conexões já abertas do pool continuam com a sessão antiga até reconectar (o app
  reconecta em seguida).
