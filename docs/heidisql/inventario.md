# Inventário — funcionalidades do HeidiSQL no Database Studio

Este documento percorre, seção por seção, o inventário de funcionalidades do HeidiSQL
([`referencia-heidisql.md`](referencia-heidisql.md), levantado do código-fonte em Delphi) e registra o status de
cada item no Database Studio depois das 8 etapas de paridade. A coluna "Onde/observação" indica a etapa em que o
item foi feito (ver `etapa-N-*.md`) e/ou o arquivo principal; quando o item não foi feito, ela traz o motivo.
O foco do projeto é MySQL/MariaDB no desktop (Wails + Go + Vue), então recursos de outros bancos, exclusivos do
Windows ou que dependem de serviços externos ficam de fora de propósito. Os itens foram conferidos nos registros
das etapas e, quando havia dúvida, no código (`internal/`, `frontend/src/`).

## Legenda

| Status | Significado |
|---|---|
| ✅ feito | Implementado com comportamento equivalente ao do HeidiSQL |
| ➕ melhorado | Implementado e melhorado em relação ao HeidiSQL (a observação diz como) |
| 🟡 parcial | Implementado em parte (a observação diz o que falta) |
| ❌ não feito | Não implementado (com o motivo) |
| ➖ não se aplica | Não faz sentido na arquitetura do Database Studio (com o motivo) |

## Resumo

| Status | Itens |
|---|---|
| ✅ feito | 175 |
| ➕ melhorado | 64 |
| 🟡 parcial | 50 |
| ❌ não feito | 83 |
| ➖ não se aplica | 17 |
| **Total** | **389** |

---

## 0. Arquitetura básica (provedor SQL, versões, tipos)

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Modelos de consulta do provedor MySQL (DROP, TRUNCATE, RENAME, KILL, SHOW …) | ✅ feito | etapa 1; `internal/database/mysql/objectsql.go`, SQL montado e validado no backend |
| Flags de recurso por versão (SRID, frações, INVISIBLE, DEFAULT (expr), visibilidade de índice…) | ✅ feito | etapas 1 e 3; "sabor" do servidor memorizado no connect, `tabledef.go` |
| Lista de tipos de dados (incl. UUID, INET4/6, espaciais) com regras de tamanho/UNSIGNED | ✅ feito | etapa 3; tipos vindos do backend + `TableEditor.vue` |
| Metadados de variáveis do servidor (dinâmica, escopo, valores de ENUM) | ❌ não feito | não há tabela interna; o editor de variáveis usa texto livre/ON-OFF |
| Lista de funções SQL por versão | ➕ melhorado | etapa 4; ajuda do próprio servidor (`HELP`) + lista do autocomplete, sempre na versão certa |
| Escape de valores (números, BIT `b'…'`, binários `0x…`, `_binary` no export) | ➕ melhorado | etapas 2 e 5; edições usam valores vinculados (`?`), seguro com `NO_BACKSLASH_ESCAPES` |
| Preferência "hex em minúsculas" | ❌ não feito | hex sempre em maiúsculas; baixo valor |

## 1. Gerenciador de sessões

### Lista de sessões

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Árvore de sessões e pastas | ➕ melhorado | etapa 7; subpastas (`Clientes/ACME`) com contagem, na própria árvore de objetos |
| Renomear inline, arrastar para pastas | 🟡 parcial | etapa 7; "Mover para a pasta…"/"Renomear pasta…" pelo menu, sem arrastar e soltar |
| Nova sessão/pasta, salvar, duplicar, excluir | ✅ feito | `ConnectionDialog.vue`, menu "Duplicar"/"Excluir…" em `ConnectionTree.vue` |
| "Pastas no topo" e caixa de filtro | 🟡 parcial | filtro regex da árvore serve às conexões; sem a opção de ordenação de pastas |
| Colunas Host, Porta, Usuário, Versão, Última conexão, Contador, Comentário | 🟡 parcial | etapa 7; dados no diálogo (aba Avançado) e dica da árvore, não em colunas |
| Fundo da linha com a cor da sessão | ✅ feito | cor por perfil (ou do ambiente) em `ConnectionDialog.vue` |
| Menu "Mais": preferências, importar/exportar configurações | ✅ feito | etapas 7 e 8 |
| Menu "Mais": verificar atualizações, ajuda, sobre | ❌ não feito | atualização depende de serviço externo; só a versão aparece na barra de status |

### Aba Configurações

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Tipo de rede MariaDB/MySQL TCP/IP | ✅ feito | `internal/database/mysql/driver.go` |
| Tipo de rede named pipe | ➖ não se aplica | exclusivo do Windows; equivalente Unix (socket) feito na etapa 7 |
| Tipo de rede SSH tunnel | ➕ melhorado | etapa 7; cliente SSH embutido (ver aba SSH) |
| ProxySQL Admin e MySQL on RDS | ❌ não feito | fora do escopo (só MySQL/MariaDB padrão) |
| MSSQL, PostgreSQL, SQLite, Interbase/Firebird | ❌ não feito | outros bancos, fora do escopo |
| Biblioteca cliente (DLL) | ➖ não se aplica | exclusivo do Windows; o driver Go é puro, sem DLL |
| Host, usuário, senha, porta | ✅ feito | `ConnectionDialog.vue`; senha no keyring do sistema quando pedido |
| Pedir credenciais ao conectar | ✅ feito | sem senha salva, a conexão pede a senha (com "salvar no keyring") |
| Autenticação do Windows | ❌ não feito | exclusivo do Windows |
| Protocolo compactado | 🟡 parcial | parâmetro `compress` em "Parâmetros" (Avançado), sem caixa própria |
| Databases (lista separada por `;`, só esses na árvore) | 🟡 parcial | etapa 7; aceita padrões `*`/`?`, mas sem o botão "…" com checklist de `SHOW DATABASES` |
| Comentário | ✅ feito | etapa 7; aparece na dica da conexão |

### Aba Túnel SSH

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Usar túnel SSH, host + porta, usuário, senha | ➕ melhorado | etapa 7; `internal/sshtunnel`, sem plink/ssh externo |
| Arquivo de chave privada | ➕ melhorado | etapa 7; OpenSSH/PEM com frase secreta e também agente SSH (`SSH_AUTH_SOCK`) |
| Executável SSH (plink/OpenSSH) | ➖ não se aplica | cliente embutido, não há executável |
| Timeout do SSH | ✅ feito | usa o tempo limite de conexão do perfil |
| Porta local (incrementa se ocupada) | ➖ não se aplica | nenhuma porta local é aberta: canal `direct-tcpip` por conexão |
| `StrictHostKeyChecking=no` | ➕ melhorado | etapa 7; verificação de chave sempre ligada (known_hosts, impressão digital SHA256, chave trocada recusada) |

### Aba SSL

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Usar SSL, chave privada, certificado, CA | ✅ feito | `driver.go` (`buildTLS`) |
| Cipher SSL | ❌ não feito | Go escolhe as cifras; baixo valor |
| Modo de verificação (nenhuma / CA / CA + host) | ➕ melhorado | 5 modos (desligado, preferido, obrigatório, verificar CA, verificar tudo) + nome do servidor |
| Versões de TLS permitidas | ➕ melhorado | mínimo TLS 1.2 |
| Aviso "SSL não usado" | ✅ feito | barra de status mostra "TLS"/"sem TLS" com a cifra |

### Aba Avançado

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Script de inicialização | ➕ melhorado | etapa 7; roda em cada conexão física do pool, falha recusa a conexão, validado ao salvar |
| Timeout de consulta/conexão | ✅ feito | tempo limite de conexão no perfil + tempo limite de consulta nas preferências |
| Ping a cada X segundos (keep-alive) | ✅ feito | etapa 7 |
| Cor de fundo da sessão | ➕ melhorado | cor + ambiente (local/dev/staging/produção) com regras de segurança |
| Padrão para ocultar databases (regex) | 🟡 parcial | etapa 7; só filtro de inclusão por padrões, sem exclusão por regex |
| Log de consultas em arquivo (DDL/DML) | ❌ não feito | o log de SQL fica em memória; histórico em `history.json` |
| Usar o fuso horário do cliente | 🟡 parcial | possível com o parâmetro `time_zone` ou o script inicial, sem caixa própria |
| Obter status completo das tabelas | ➖ não se aplica | sempre lido do `information_schema` com estatísticas atualizadas (etapa 1) |
| Autenticação em texto claro | ➕ melhorado | só aceita junto com TLS obrigatório |
| Forçar Unicode | ➖ não se aplica | conexão sempre em utf8mb4 (ou charset escolhido em parâmetros) |

### Estatísticas e conexão

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Estatísticas: criado, última conexão, sucessos, falhas | ✅ feito | etapa 7; mantidas pelo app em `connections.json` |
| Flags do cliente, `program_name`, LOCAL_INFILE | ➕ melhorado | `multiStatements` desligado; `LOCAL INFILE` liberado só para o arquivo escolhido (etapa 5) |
| Senha expirada (erro 1820) com diálogo de troca | ❌ não feito | a troca existe só pelo "Alterar a minha senha" (etapa 6) |
| Versão (`version` + `version_comment`) | ✅ feito | barra de status e aba do servidor |
| Charset utf8mb4 com fallback | ✅ feito | padrão do driver; charset configurável em parâmetros |
| Uptime e hora do servidor no connect | 🟡 parcial | uptime na aba do servidor; sem relógio do servidor |
| Cache das tabelas do `information_schema`, thread id | ➖ não se aplica | o app consulta direto; cada aba tem sua conexão |
| Reconexão automática | ✅ feito | pool do `database/sql` com verificação de vivacidade |
| Detecção de `USE x` | ✅ feito | database atual da aba atualizado após a execução (`query/service.go`) |
| Último database usado por sessão | 🟡 parcial | abas restauradas guardam o database; perfil tem "database inicial"; sem a preferência |

## 2. Árvore de databases

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Tipos de nó: sessão, database, grupos, tabela, view, rotinas, trigger, evento, coluna | ➕ melhorado | etapa 1; também índices sob a tabela e pastas de conexões |
| Coluna de tamanho (sessão, database, tabela) | ❌ não feito | tamanhos aparecem nas abas do servidor e do database, não na árvore |
| Carga de objetos por database (status, rotinas, triggers, eventos) | ✅ feito | etapa 1; falha isolada se faltar privilégio |
| Cache por database e por tabela | ✅ feito | `stores/metadata.ts`, `internal/metadata` |
| Filtros de database e de tabela (regex, `;` = OU) | 🟡 parcial | etapa 1; um único filtro regex para tudo |
| Histórico dos filtros da árvore | ❌ não feito | baixo valor |
| Favoritos e "mostrar só favoritos" | ❌ não feito | não priorizado |
| Editar (abre o editor; Alter Database no database) | ✅ feito | etapas 1 e 3 |
| Excluir, esvaziar, selecionar N linhas, executar rotina | ✅ feito | etapa 1; `ConnectionTree.vue` |
| Selecionar N linhas com Shift qualificando `db.t` | ❌ não feito | detalhe menor |
| Criar novo: database, tabela, cópia de tabela, view, procedure, function, trigger, evento | ✅ feito | etapas 1, 3 e 6 |
| Limpar filtro e ordenação da aba de dados | ✅ feito | etapa 2; "Remover filtro" na grade |
| Exportar SQL, manutenção, buscar texto, edição em massa, gerar dados | ✅ feito | etapas 5 e 6; submenu "Ferramentas" |
| Expandir tudo / recolher tudo | 🟡 parcial | só "Recolher tudo" |
| Opções: agrupar por tipo, colunas sob tabelas | ✅ feito | sempre ligadas |
| Opções: exibir tamanho, expandir ao clicar, duplo clique insere o texto | ➖ não se aplica | duplo clique abre o objeto; inserir nome fica no menu |
| Imprimir | ❌ não feito | impressão fora do escopo |
| Atualizar (F5) e "Full status refresh" | ➕ melhorado | F5; estatísticas sempre reais (`information_schema_stats_expiry = 0`) |
| Propriedades da conexão, desconectar | ✅ feito | menu da conexão |
| SQLite: anexar/desanexar database | ❌ não feito | outro banco |
| DROP de database com confirmação crítica | ➕ melhorado | etapa 1; em produção é preciso digitar o nome da conexão |
| DROP de objetos com `FOREIGN_KEY_CHECKS=0` e restauração | ✅ feito | etapa 1; `query.Service.RunAdmin` |
| Esvaziar tabelas com opção de desligar FKs | ✅ feito | etapa 1 |
| Executar rotinas (pede parâmetros, gera CALL/SELECT) | ➕ melhorado | etapa 1; também variáveis OUT |
| Renomear (`RENAME TABLE`) | ✅ feito | etapa 1; F2/menu com diálogo (também eventos) |

## 3. Aba do servidor (Host)

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Filtro em cada lista | ✅ feito | etapa 1; regex com `;` (sem histórico) |
| Atualização automática com intervalo | ✅ feito | processos, status e comandos (1 a 30 s) em `HostView.vue` |
| Databases: tamanho, tabelas, views, última alteração, collation | ✅ feito | etapa 1; com barras e charset |
| Databases: itens, functions, procedures, triggers, events e "Buscar itens" | ❌ não feito | contagens por tipo ficam na aba do database |
| Variáveis: sessão × global com destaque | ➕ melhorado | etapa 1; filtro "sessão ≠ global" |
| Editor de variável (tipo, escopo, `SET @@global/session`) | 🟡 parcial | etapa 1; edita só GLOBAL (SESSION abre uma aba), sem detecção de ENUM/dinâmica |
| `SHOW WARNINGS` depois do SET | ❌ não feito | erros aparecem na mensagem; avisos não são lidos aqui |
| Status: média por hora/segundo, bytes formatados | ✅ feito | etapa 1 |
| Processos: `information_schema.PROCESSLIST` com colunas extras | ➕ melhorado | etapa 1; `PROGRESS` do MariaDB, ocultar ociosos |
| Processos: ícones de estado e barra de tempo | 🟡 parcial | barra e destaque por comando, sem ícones |
| Painel da instrução com EXPLAIN | ✅ feito | etapa 1 |
| Kill de processos (o HeidiSQL executa `KILL QUERY`) | ➕ melhorado | etapa 1; `KILL QUERY` e `KILL` separados, em lote |
| Estatísticas de comandos (total, médias, percentual com barra) | ✅ feito | etapa 1 |
| FLUSH HOSTS/LOGS/PRIVILEGES/TABLES/STATUS | ➕ melhorado | etapa 6; também `USER_RESOURCES` e `QUERY CACHE` |
| `FLUSH TABLES WITH READ LOCK` + "faça o backup agora" | ❌ não feito | não priorizado (o dump usa snapshot consistente) |
| Variáveis/processos de MSSQL e PostgreSQL | ❌ não feito | outros bancos |

## 4. Aba do database (lista de objetos)

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Colunas nome, linhas, tamanho, datas, engine, comentário, tipo (com barras) | ✅ feito | etapa 1; `DatabaseView.vue`, também rotinas, triggers e eventos |
| Colunas extras (formato, médias, índices, livre, auto incr., collation, opções…) | 🟡 parcial | etapa 1; "mais colunas", sem Version, Max data length e Checksum |
| Destaque de espaço livre | ➕ melhorado | etapa 1; quando > 30% e > 10 MiB |
| Texto de status (N objetos) e "Selecionados: N" | ✅ feito | etapa 1 |
| Renomear e abrir com duplo clique | ✅ feito | renomear por diálogo, não inline |
| Visibilidade/largura/ordem das colunas persistidas | 🟡 parcial | ordenação e redimensionamento, sem persistência por coluna |
| Menu do cabeçalho e "Copiar todas as linhas como CSV" | 🟡 parcial | copia as linhas selecionadas como TSV (Ctrl+A, Ctrl+C) |

## 5. Editor de tabela

### Abas e opções

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Abas Básico, Opções, Índices, FKs, CHECK, Partições, CREATE/ALTER | ✅ feito | etapa 3; `TableEditor.vue` |
| Auto increment, collation padrão com "converter dados" | ✅ feito | etapa 3; `CONVERT TO CHARACTER SET` |
| Engine a partir de `SHOW ENGINES` | ✅ feito | etapa 3 |
| Average row length, max rows, checksum, row format | ✅ feito | etapa 3 |
| UNION e INSERT_METHOD (engine MERGE) | ❌ não feito | engine MERGE pouco usada |
| Comentário da tabela | ✅ feito | etapa 3 |

### Grade de colunas

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Nome, tipo, tamanho, UNSIGNED, NULL, ZEROFILL, padrão, comentário, collation | ✅ feito | etapa 3 |
| Expressão, virtualidade (VIRTUAL/STORED/PERSISTENT), INVISIBLE | ✅ feito | etapa 3; nos dois servidores conforme a versão |
| SRID | 🟡 parcial | lido e preservado pelo backend (`tabledef.go`), sem campo para editar |
| COMPRESSED (MariaDB) | ❌ não feito | recurso específico e raro |
| Regras de edição (UNSIGNED só numéricos, NULL travado em PK…) | ✅ feito | etapa 3; validado também no backend |
| Editor de padrão (sem padrão, texto, NULL, expressão, AUTO_INCREMENT, ON UPDATE) | ✅ feito | etapa 3 |
| Troca de tipo ajusta tamanho e atributos | ✅ feito | etapa 3 |
| Adicionar (copia tipo da coluna focada), remover, subir/descer | ➕ melhorado | etapa 3; remover limpa partes de índice e renomear atualiza índices/FKs |
| Arrastar para reordenar | ❌ não feito | há subir/descer |
| Copiar/colar colunas | ❌ não feito | não priorizado |
| Criar índice a partir da coluna / adicionar a um índice | ✅ feito | etapa 3; menu de contexto |
| SQL de coluna na ordem do MySQL (incl. `DEFAULT (expr)`) | ✅ feito | etapa 3; `tabledef.go` |
| Leitura de colunas e decisão literal × expressão | ➕ melhorado | etapa 3; regras de MySQL 8 e MariaDB, teste de ida e volta com zero ALTERs |

### Índices, FKs, CHECK e partições

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Leitura de índices (incl. funcionais e visibilidade) | ✅ feito | etapa 3; `information_schema.STATISTICS` |
| Tamanho dos índices (`innodb_index_stats`) | ❌ não feito | só o total de índices na aba do database |
| Tipo, algoritmo, comentário, direção, visibilidade, prefixo | ✅ feito | etapa 3 |
| FKs: colunas, tabela/database referenciados, ações | ✅ feito | etapa 3 |
| FKs: nome automático | ✅ feito | `fk_<tabela>_<ref>` em `tabledef.go` |
| FKs: validação de número e tipo das colunas | 🟡 parcial | validação estrutural no backend; compatibilidade de tipos fica com o servidor |
| Botão "Reverso" (tabelas que referenciam) | ➕ melhorado | etapa 1; seção "Referenciada por" com as FKs completas |
| CHECK: nome e expressão (MySQL e MariaDB) | ✅ feito | etapa 3 |
| Partições (só no CREATE) | ➕ melhorado | etapa 3; alterar gera o ALTER e apagar gera `REMOVE PARTITIONING` |
| CREATE de tabela nova | ✅ feito | etapa 3 |
| Geração do ALTER na ordem do HeidiSQL | ➕ melhorado | etapa 3; diff contra a definição atual do servidor, recusa se mudou, ALTER mínimo |
| Erro "constraint incorrectly formed" com trecho do `SHOW ENGINE INNODB STATUS` | ❌ não feito | o erro do servidor é mostrado com a instrução |

## 6. Outros editores de objetos

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| View: nome, definer, SQL SECURITY, algoritmo, CHECK OPTION, SELECT | ✅ feito | etapa 3; `ViewEditor.vue` |
| View: renomear com `RENAME TABLE` | ✅ feito | etapa 3 |
| View: formatação original via `LOAD_FILE(.frm)` | ➕ melhorado | o SELECT é formatado localmente ao abrir, sem ler arquivos do servidor |
| Rotina: nome, tipo, retorno, acesso a dados, segurança, determinístico, comentário, definer | ✅ feito | etapa 3; `RoutineEditor.vue` |
| Rotina: grade de parâmetros (IN/OUT/INOUT, reordenar) | ✅ feito | etapa 3 |
| Rotina: corpo, aba CREATE, botão Executar | ✅ feito | etapa 3; executar pela árvore |
| Rotina: salvar com rotina temporária para validar | ➕ melhorado | etapa 3; restaura a versão anterior se a criação final falhar |
| Trigger: nome, definer, tabela, momento, evento, corpo | ✅ feito | etapa 3; `TriggerEditor.vue` |
| Trigger: salvar com restauração em falha | ➕ melhorado | etapa 3; valida antes com cópia temporária |
| Evento: agenda (uma vez / a cada N), início/fim, preservar, estado, comentário, definer | ✅ feito | etapa 3; `EventEditor.vue` |
| Evento: `ALTER EVENT … RENAME TO` | ✅ feito | etapa 3 |
| Criar database com collation | ✅ feito | `CreateDatabaseDialog.vue` |
| Alterar collation do database | ✅ feito | etapa 1; aba do servidor |
| Renomear database (emulado com RENAME TABLE) | ❌ não feito | operação arriscada; não priorizada |
| Definer sugerido a partir de `mysql.user` | ✅ feito | etapa 3 |

## 7. Aba de dados (grade)

### Leitura

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Colunas-chave sempre selecionadas | ✅ feito | etapa 2 |
| Carga parcial de colunas longas e valor completo sob demanda | ➕ melhorado | etapa 2; indicador exato de truncamento (`CHAR_LENGTH`) |
| `SELECT … WHERE … ORDER BY … LIMIT` montado no backend | ✅ feito | etapa 2; `database.BrowseSpec` |
| Páginas de N linhas, "próximas" e "tudo" (até 100.000) | ✅ feito | etapa 2; carga cancelável com `KILL QUERY` |
| Rótulo com total aproximado/exato e "contar" | ✅ feito | etapa 2; botão Contar |

### Filtro e ordenação

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Caixa WHERE com histórico de 20 por tabela | ✅ feito | etapa 2 |
| Busca de texto em todas as colunas | ✅ feito | etapa 2; ignora binários |
| Filtro, ordenação e colunas lembrados por tabela | ✅ feito | etapa 2; sempre ligado |
| Clique no cabeçalho alterna ASC → DESC → nenhum | ✅ feito | etapa 2 |
| Diálogo de ordenação por várias colunas | 🟡 parcial | etapa 2; Shift+clique no cabeçalho, sem diálogo |
| Ordenação local nas grades de consulta | ✅ feito | etapa 4 |
| Seletor de colunas (todas, filtro) | 🟡 parcial | etapa 2; sem "ordem alfabética" e coluna de número da linha |
| Filtros rápidos pelos valores focados (=, ≠, >, <, LIKE, IN, NOT IN) | ✅ feito | etapa 2 |
| Filtros rápidos com valor digitado | 🟡 parcial | "Igual a…" e "Contendo…", sem ≠, >, < |
| IS NULL / IS NOT NULL | ✅ feito | etapa 2 |
| Filtros a partir da área de transferência | ❌ não feito | não priorizado |
| Shift+clique combina com AND | ✅ feito | submenu "Adicionar ao filtro (AND)" |
| Valores mais frequentes | ✅ feito | etapa 2 |
| Remover filtro | ✅ feito | etapa 2 |

### Edição

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Identificação da linha (PK, UNIQUE NOT NULL, senão todas + LIMIT 1) | ➕ melhorado | etapa 2; aviso e recusa se afetar mais de uma linha |
| WHERE com `CAST(… AS CHAR)` para tipos inexatos | ✅ feito | etapa 2 |
| UPDATE / INSERT (com `LAST_INSERT_ID`) / DELETE | ➕ melhorado | etapa 2; valores vinculados e gravação em lote numa única transação (Ctrl+S) |
| Inserir linha e duplicar sem chaves | ✅ feito | etapa 2; Insert, Ctrl+D |
| Duplicar linha com chaves | ❌ não feito | só a duplicação sem colunas AUTO_INCREMENT/geradas |
| Gravar, cancelar, excluir, definir NULL | ✅ feito | etapa 2; mais desfazer da linha (Ctrl+Z) |
| Valor como função SQL (sem aspas) | ✅ feito | etapa 2; modo "Expressão SQL" no editor |
| Editar sem lookup de chave estrangeira | ✅ feito | "Editar em janela…" usa texto livre |
| Inserir valor: data/hora local e UTC, data, hora, ano, timestamp UNIX, UUID | ✅ feito | `utils/grid.ts`, avaliados pelo servidor |
| Coluna "timestamp UNIX" exibida como data | 🟡 parcial | etapa 8; só exibição (edição, filtros e cópia continuam com o número) |
| Salvar BLOB em arquivo | ✅ feito | etapa 5 |
| Prévia de imagem | ✅ feito | etapa 2; PNG/JPEG/GIF/WebP/BMP |
| Ver binário como texto | ✅ feito | etapa 2 |
| Abrir URL | ❌ não feito | não priorizado |
| Seguir chave estrangeira | ✅ feito | etapa 2; abre a tabela já filtrada |
| Copiar, "copiar como" formato de exportação, colar | ➕ melhorado | etapas 2 e 5; colar TSV em várias células, copiar WHERE da linha |

### Editores de célula

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Lista de valores de FK com rótulo de texto | ✅ feito | etapa 2 |
| Lista para ENUM/BOOL e caixas para SET | ✅ feito | etapa 2 |
| Editor de data/hora | 🟡 parcial | etapa 2; sem preenchimento automático com "agora" (há "Inserir valor") |
| Editor hexadecimal com carregar arquivo | ✅ feito | etapas 2 e 5 |
| Editor de texto em janela (quebra, estilo de fim de linha, carregar, realce, formatar) | 🟡 parcial | etapa 2; JSON com validação/formatação, estilo de quebra só exibido, sem escolha de realce |
| Inserir arquivos (diálogo com marcadores `%filename%` etc.) | 🟡 parcial | etapa 5; "Carregar arquivo…" numa célula, sem o diálogo de vários arquivos |

## 8. Aba de consulta

### Execução

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Várias abas: nova, fechar, fechar todas, fechar à direita, renomear | ✅ feito | etapa 4; `TabBar.vue` |
| Nova aba em segundo plano | ❌ não feito | detalhe menor |
| Abas restauradas ao iniciar (com conteúdo não salvo) | ✅ feito | `workspace.json` |
| Aba ligada a arquivo: abrir, salvar, salvar como | ✅ feito | etapa 8 (atalhos Ctrl+O/S/Shift+S) |
| Salvar seleção | ❌ não feito | detalhe menor |
| Executar, executar seleção, executar instrução sob o cursor | ✅ feito | Ctrl+Shift+Enter/F5, Ctrl+Enter |
| EXPLAIN da instrução atual | ➕ melhorado | etapa 4; também FORMAT=JSON/TREE, EXPLAIN ANALYZE ou ANALYZE |
| Modo "enviar lote de uma vez" | ❌ não feito | etapa 4; `multiStatements` desligado por segurança |
| Parar em erros | 🟡 parcial | o script do editor sempre para no primeiro erro; opção só na execução de arquivo |
| Divisor de instruções com comentários, aspas e `DELIMITER` | ➕ melhorado | etapas 4 e 5; comentários executáveis `/*! */` e modo incremental |
| Ação "definir delimitador" | ➖ não se aplica | basta usar `DELIMITER` no texto |
| Aviso de UPDATE/DELETE sem WHERE | ✅ feito | preferência de confirmação de operações destrutivas |
| Um resultado por instrução | ✅ feito | seletor de resultados com dica do SQL |
| Limite de conjuntos de resultados | ❌ não feito | não há limite configurável |
| Nome do resultado por `-- names:` | ❌ não feito | detalhe menor |
| Ícones de chave no cabeçalho do resultado | ❌ não feito | não priorizado |
| Resultados editáveis | ✅ feito | etapa 4; `api.EditableResult` |
| Linha de log com linhas afetadas, duração e avisos | ➕ melhorado | etapa 4; avisos de `SHOW WARNINGS` exibidos sobre o resultado, não só no log |
| Navegação até o erro | ✅ feito | marcadores na linha da instrução com erro (`SqlEditor.vue`) |

### Painel de ajuda

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Colunas da tabela com geradores SELECT/INSERT/UPDATE/DELETE | ✅ feito | etapa 4; `QueryHelpers.vue` |
| Funções SQL agrupadas | ✅ feito | etapa 4 |
| Palavras-chave | ✅ feito | etapa 4 |
| Snippets em pasta (inserir, carregar, renomear, excluir, abrir pasta) | ➕ melhorado | etapa 4; também criar a partir da seleção |
| Histórico de consultas por sessão (data, db, duração, SQL) | ✅ feito | `HistoryPanel.vue`, com busca |
| Histórico: agrupado por dia, manter N dias, só sucessos, sem duplicatas | 🟡 parcial | limite por número de entradas, sem agrupamento por dia |
| Limpar histórico | ✅ feito | painel e preferências |
| Perfil da consulta (`SHOW PROFILE`) | ✅ feito | etapa 4 |
| Parâmetros `:nome` | ➕ melhorado | etapa 4; ignora strings, comentários e `:=`, liga sozinho, valores só em memória |

### Recursos do editor

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Autocompletar | ✅ feito | `internal/autocomplete`, ligável nas preferências (etapa 8) |
| Autocompletar: intervalo configurável | ❌ não feito | Ctrl+Espaço ou sugestão imediata |
| Palavras-chave em maiúsculas ao digitar | ➕ melhorado | etapa 8; ignora strings, crases, comentários, `t.from`, `@set` |
| Dobrar código, comentar linha, mover linha | ✅ feito | recursos do Monaco |
| Quebra de linha, tabulação para espaços | ✅ feito | etapa 8 |
| Zoom com Ctrl+roda | ❌ não feito | tamanho da fonte nas preferências |
| Submenu "inserir função" | 🟡 parcial | duplo clique na lista de funções do painel de ajuda |
| Localizar/substituir (maiúsculas, palavra, regex, seleção, substituir tudo) | ✅ feito | Monaco (Ctrl+F/Ctrl+H) |
| Localizar: "perguntar ao substituir" | ❌ não feito | não existe no Monaco |
| Formatador interno | ➕ melhorado | `sql-formatter` local (`editor/format.ts`) |
| Formatadores online (heidisql.com, sqlformat.org) | ❌ não feito | enviariam o SQL para terceiros |
| Ajuda SQL (F1, `HELP 'contents'`) | ➕ melhorado | etapa 4; busca por prefixo e tópico da palavra sob o cursor |
| Sequal Suggest (IA) | ❌ não feito | depende de serviço externo |

### Arquivos e log

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Executar arquivo SQL grande sem abrir no editor | ➕ melhorado | etapa 5; blocos de 4 MiB, `.sql.gz`, em tarefa cancelável |
| Pergunta "executar ou carregar" para arquivos > 5 MB | ➖ não se aplica | ações separadas: abrir no editor ou executar arquivo |
| Arquivos recentes com escolha de codificação | ❌ não feito | arquivos são lidos em UTF-8 |
| Log de SQL com categorias (erros, usuário, interno…) | ➕ melhorado | etapa 1; todo SQL do pool, filtro por conexão, regex, "só erros", "ocultar catálogo" |
| Log: últimas N linhas | ✅ feito | etapa 8; 100 a 100.000 entradas |
| Log: cortar linhas, data/hora, barra horizontal | 🟡 parcial | hora sempre exibida; sem corte configurável |
| Log: copiar linha para nova aba, limpar | ✅ feito | duplo clique abre a instrução numa aba |
| Log: salvar em arquivo, gravar em arquivo, abrir pasta | ❌ não feito | há "copiar entradas visíveis" |
| Arquivos de log DDL/DML por sessão | ❌ não feito | ver "Log de consultas em arquivo" |

## 9. Exportar linhas da grade

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Linhas selecionadas / carregadas | ✅ feito | etapa 5; `ExportDialog.vue` |
| Todas as linhas | ➕ melhorado | etapa 5; reexecuta e grava em streaming, sem limite e sem truncar |
| Destino: área de transferência ou arquivo | ✅ feito | etapa 5 |
| Arquivo: codificação, "abrir após criar", recentes | 🟡 parcial | UTF-8 (BOM no CSV do Excel); painel de tarefas abre a pasta |
| Formatos texto: CSV Excel, CSV, HTML, XML, LaTeX, Textile, Jira, PHP, Markdown, JSON, JSON Lines | ➕ melhorado | etapa 5; `internal/gridexport`, mais TSV, precisão de inteiros grandes no JSON |
| Formatos SQL: INSERT, INSERT IGNORE, REPLACE, DELETE+INSERT, UPDATE | ✅ feito | etapa 5; INSERT multi-linha |
| Separador, delimitador, fim de linha, texto do NULL | ✅ feito | etapa 5 (sem os atalhos de caracteres especiais) |
| Incluir nomes das colunas, coluna auto incremento | ✅ feito | etapa 5 |
| Incluir a consulta SQL | 🟡 parcial | JSON leva tabela/consulta; demais formatos não |
| Remover quebras de linha | ✅ feito | etapa 5 |
| Só a coluna focada | ❌ não feito | não priorizado |
| Configurações separadas para a área de transferência | ➖ não se aplica | as opções são lembradas por formato |
| "Copiar como" no menu da grade | ✅ feito | "Copiar → Em outro formato…" |

## 10. Ferramentas de tabela

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Árvore de databases/objetos com marcar todos/nenhum | 🟡 parcial | etapa 6; `TableToolsDialog.vue`, sem filtros regex, inverter e tamanho da seleção |
| Manutenção CHECK/ANALYZE/CHECKSUM/OPTIMIZE/REPAIR com opções | ➕ melhorado | etapa 6; opções validadas no backend, em tarefa, erros por tabela não param |
| Buscar texto: tipos de coluna, modos, maiúsculas | ✅ feito | etapa 6 |
| Buscar texto: expressão regular | ➕ melhorado | etapa 6; valores vinculados e `%`/`_` literais |
| Buscar texto: aba SQL com condição livre | ❌ não feito | não priorizado |
| Buscar texto: ocorrências em rotinas | ❌ não feito | só tabelas e views |
| Buscar texto: ver resultados | ➕ melhorado | duplo clique abre a tabela já filtrada |
| Dump: DROP/CREATE DATABASE e de objetos | ✅ feito | etapa 5; `internal/dump` |
| Dump: modos de dados (nenhum, DELETE+INSERT, INSERT, IGNORE, REPLACE) | ✅ feito | etapa 5 |
| Dump: tamanho máximo do INSERT | ✅ feito | etapa 5 |
| Dump: arquivo, compactado, pasta por objeto | ✅ feito | etapa 5; `.sql.gz` no lugar de ZIP |
| Dump: outro database / outro servidor | ➕ melhorado | etapa 5; corrige views e triggers qualificados com o database de origem |
| Dump: área de transferência | ❌ não feito | dumps grandes; não priorizado |
| Dump: marcadores no nome do arquivo (`%db`, `%date`…) | ❌ não feito | o nome é escolhido no diálogo nativo |
| Dump: comentários, transação, remover AUTO_INCREMENT e DEFINER | ✅ feito | etapa 5 |
| Dump: comando mysqldump equivalente | ✅ feito | etapa 5 |
| Dump: cabeçalho/rodapé de sessão | ✅ feito | etapa 5 |
| Dump: leitura dos dados | ➕ melhorado | etapa 5; snapshot consistente (como `--single-transaction`) |
| Dump: binários `_binary`, colunas geradas fora, MRG/FEDERATED sem dados | ✅ feito | etapa 5 |
| Dump: views provisórias, rotinas com DELIMITER, triggers por último | ✅ feito | etapa 5 |
| Edição em massa: mover, engine, charset, collation, AUTO_INCREMENT | ✅ feito | etapa 6; prévia, cada tabela tentada mesmo se outra falhar |
| Gerar dados aleatórios | ➕ melhorado | etapa 6; faixas por tipo, ENUM/SET, JSON, até 1.000.000 linhas em tarefa |

## 11. Importação CSV

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Arquivo e codificação | 🟡 parcial | etapa 5; UTF-8 e Latin-1/Windows-1252 |
| Separador, delimitador, escape, fim de linha | ➕ melhorado | etapa 5; detectados automaticamente com prévia |
| Ignorar N linhas | ✅ feito | etapa 5 |
| Baixa prioridade (`LOW_PRIORITY`) | ❌ não feito | pouco usado |
| Números no formato local | ✅ feito | etapa 5; só nas colunas numéricas |
| Esvaziar a tabela antes | ✅ feito | etapa 5 |
| Manter o diálogo aberto | ➖ não se aplica | a importação roda como tarefa em segundo plano |
| Duplicadas: INSERT / IGNORE / REPLACE | ✅ feito | etapa 5 |
| Método: servidor (`LOAD DATA`) ou cliente | ➕ melhorado | etapa 5; cliente com valores vinculados, "tudo ou nada", `LOCAL INFILE` só do arquivo escolhido |
| Destino e mapeamento de colunas | ✅ feito | etapa 5 |
| Detector de layout e tipos + CREATE TABLE | ➕ melhorado | etapa 5; tabela nova criada pelo editor de tabela do backend |

## 12. Gerenciador de usuários

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Lista de contas e roles | ✅ feito | etapa 6; `UserManager.vue` |
| Ícones de aviso (sem senha, hash inválido, `skip_name_resolve`, grant desconhecido) | 🟡 parcial | sinaliza sem senha e bloqueada |
| Filtro, adicionar usuário/role, clonar, excluir | ✅ feito | etapa 6; clonar não copia a senha |
| Credenciais: usuário, host com sugestões, senha | ✅ feito | etapa 6; senha em branco = manter |
| Gerador de senha | ✅ feito | etapa 6; 16 caracteres |
| Plugin de autenticação | ✅ feito | etapa 6; lista do servidor |
| Role padrão | ✅ feito | etapa 6 |
| Limites por hora e conexões simultâneas | ✅ feito | etapa 6 |
| REQUIRE NONE/SSL/X509/cipher, issuer, subject | ✅ feito | etapa 6 |
| Privilégios por objeto (global, database com curingas, tabela, rotina, coluna) | ➕ melhorado | etapa 6; privilégios de `SHOW PRIVILEGES` (inclui dinâmicos do MySQL 8) |
| Cores por tipo de privilégio | ❌ não feito | detalhe visual |
| Roles com ADMIN OPTION | ✅ feito | etapa 6; MySQL 8 e MariaDB |
| Leitura por `SHOW GRANTS` e `SHOW CREATE USER` | ✅ feito | etapa 6; `users.go` |
| Salvar: CREATE/RENAME/ALTER USER, GRANT/REVOKE, roles | ➕ melhorado | etapa 6; diff contra o servidor, senhas mascaradas em prévia e log |
| Suporte a servidores anteriores ao 5.0.2 | ➖ não se aplica | servidores alvo são MySQL 8 e MariaDB 10+ |
| Expiração de senha e bloqueio de conta | ➕ melhorado | etapa 6; não existe no HeidiSQL |
| Diálogo de troca de senha | 🟡 parcial | etapa 6; "Alterar a minha senha" (atualiza o keyring), não no erro de senha expirada |

## 13. Copiar tabela, selecionar objeto, sincronização

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Copiar tabela: destino, colunas, índices, FKs, dados com WHERE | ➕ melhorado | etapa 6; também CHECKs e nomes de FK/CHECK únicos |
| Copiar tabela: substituir se existir | ✅ feito | etapa 6 |
| Seletor de objeto do banco (com curingas) | ✅ feito | etapa 6; campos com sugestões no gerenciador de usuários |
| Sincronizar databases (inacabado no HeidiSQL) | ➕ melhorado | etapa 6; "Comparar estruturas" com ALTER/CREATE/DROP e aplicação |

## 14. Preferências

### Geral

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Caminho dos programas de linha de comando do MySQL | ❌ não feito | não há lançador do cliente `mysql` |
| Idioma | ❌ não feito | interface só em português; pacotes de idioma fora do escopo |
| Pasta de snippets personalizada | ❌ não feito | pasta fixa `~/.config/dbstudio/snippets` |
| Fonte e tamanho da interface | ❌ não feito | segue o sistema |
| Tema | ➕ melhorado | claro, escuro ou seguir o sistema |
| Pacote de ícones | ❌ não feito | pacotes de ícones fora do escopo |
| URL de pesquisa na web | ❌ não feito | não priorizado |
| Reconectar às sessões abertas ao iniciar | ✅ feito | etapa 8 |
| Restaurar o último database ao iniciar | 🟡 parcial | as abas restauradas mantêm o database |
| Verificar atualizações | ❌ não feito | depende de serviço externo |
| Enviar estatísticas de uso | ❌ não feito | depende de serviço externo |
| Permitir várias instâncias | ❌ não feito | uma janela por aplicativo |
| Barras nas listas | ✅ feito | sempre ligadas |
| Zoom com Ctrl+roda | ❌ não feito | tamanho da fonte do editor nas preferências |
| Restaurar tamanhos dos painéis | ✅ feito | etapa 8 |

### Log

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Últimas N linhas | ✅ feito | etapa 8 |
| Cortar linhas em N caracteres | ❌ não feito | linhas longas são encurtadas na exibição |
| Tipos de evento | 🟡 parcial | filtros "só erros" e "ocultar catálogo" no painel |
| Gravar log em arquivo | ❌ não feito | ver seção 8 |
| Data/hora, barra horizontal | ✅ feito | hora sempre exibida |
| Histórico ligado + dias mantidos | 🟡 parcial | etapa 8; limite por número de entradas |

### SQL

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Fonte do editor e tamanho | 🟡 parcial | etapa 8; só tamanho |
| Largura da tabulação | ✅ feito | etapa 8 |
| Máximo de conjuntos de resultados | ❌ não feito | ver seção 8 |
| Cores do realce de sintaxe | ❌ não feito | etapa 8; seguem o tema |
| Autocompletar: ligar, intervalo, busca no meio | 🟡 parcial | etapa 8; liga/desliga, sem intervalo |
| Maiúsculas automáticas, tabulação para espaços | ✅ feito | etapa 8; mais quebra de linha e minimapa |

### Formatação da grade

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Largura máxima das colunas | ✅ feito | etapa 8 |
| Fonte da grade | ❌ não feito | segue o tema |
| Linhas por página e máximo | ✅ feito | preferência "linhas por página"; "Tudo" até 100.000 |
| Linhas de texto por linha da grade | ❌ não feito | uma linha por registro |
| Cores por categoria de tipo | ❌ não feito | etapa 8; seguem o tema |
| Fundo do NULL e linhas alternadas | 🟡 parcial | fixos pelo tema; sem destaque de texto igual |
| Aviso de ordenação acima de N linhas | ➖ não se aplica | ordenação local só das linhas carregadas |
| Zeros decimais máximos em floats | ❌ não feito | valores exibidos como vêm do servidor |
| Formato local de números | ✅ feito | etapa 8; separador de milhares só na exibição |
| SQL do resultado na dica da aba | ✅ feito | dica do seletor de resultados |
| Hex em minúsculas, coluna de número da linha | ❌ não feito | não priorizado |

### Editores de dados

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Estilo padrão de quebra de linha | ❌ não feito | quebras mantidas como estão |
| Ligar editores BLOB/HEX, data/hora, ENUM, SET | ➖ não se aplica | sempre ligados |
| Preencher data/hora vazia com "agora" | ❌ não feito | há "Inserir valor → Data e hora atual" |
| Lembrar filtros, ordenação e colunas | ✅ feito | sempre ligado (etapa 2) |
| Mostrar valores nas colunas de FK | ✅ feito | sempre ligado (etapa 2) |
| Clique no cabeçalho ordena | ✅ feito | sempre ligado |
| Busca incremental | ❌ não feito | não priorizado |

### Atalhos, arquivos e abas

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Atalho primário e secundário por ação | 🟡 parcial | etapa 8; um atalho por ação com detecção de conflitos; atalhos do editor/grade fixos |
| Ícones de abas em tons de cinza | ➖ não se aplica | sem ícones por aba no mesmo estilo |
| Escolha do formatador (interno/online/perguntar) | ➖ não se aplica | só o formatador local existe |
| Perguntar ao fechar arquivos alterados | ✅ feito | `stores/tabs.ts` |
| Reabrir arquivos e conteúdo não salvo | ✅ feito | `workspace.json` |
| Fechar aba com duplo clique / botão do meio | 🟡 parcial | botão do meio fecha; duplo clique renomeia |
| Restaurar padrões, aplicar | ✅ feito | etapa 8 |

## 15. Diversos

### Barra de status

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Contagem de objetos / descrição de função | 🟡 parcial | contagens e seleção na aba do database |
| Seleção ou linha:coluna do editor | ✅ feito | `StatusBar.vue` |
| Tempo conectado | ❌ não feito | não priorizado |
| Tipo e versão do servidor com detalhes da conexão | 🟡 parcial | versão e TLS/cifra; sem a dica completa |
| Uptime do servidor | 🟡 parcial | na aba do servidor |
| Hora do servidor | ❌ não feito | não priorizado |
| Ocupado/ocioso | ➕ melhorado | estado por aba, transação aberta e painel de tarefas |

### Inicialização e configurações

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Linha de comando (`-h`, `-u`, `-p`, `--psettings`…) | ❌ não feito | etapa 8; não priorizado |
| Arquivo de configurações portátil | ❌ não feito | etapa 8; configuração em `~/.config/dbstudio` |
| Exportar/importar configurações | ➕ melhorado | etapa 7; JSON sem senhas nem segredos SSH (o HeidiSQL exporta senhas ofuscadas) |
| Reconectar ao iniciar e focar a última sessão | ✅ feito | etapa 8 |
| Keep-alive e reconexão | ✅ feito | etapa 7 |

### Outras funcionalidades

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| Verificação de atualizações | ❌ não feito | depende de serviço externo |
| Abrir o cliente `mysql` na linha de comando | ❌ não feito | não priorizado |
| Imprimir lista | ❌ não feito | impressão fora do escopo |
| JumpList | ❌ não feito | exclusivo do Windows |
| Nova janela, sair, sobre, ajuda, links, doação | 🟡 parcial | só sair; versão na barra de status; várias instâncias fora do escopo |
| Copiar, copiar formatado, copiar com tabs | 🟡 parcial | Ctrl+C copia TSV; sem "copiar formatado" |
| Copiar nomes de colunas | 🟡 parcial | nome da coluna focada; TSV inclui o cabeçalho |
| Copiar todas as linhas da lista ou árvore como CSV | 🟡 parcial | listas copiam a seleção (Ctrl+A) como TSV |
| Ir para a árvore (Ctrl+D) e filtro de tabelas (Ctrl+E) | ✅ feito | etapa 8; Ctrl+Shift+D foca o filtro (Ctrl+D duplica linha) |
| Alternar editor/resultados (F6) | ✅ feito | etapa 8 |
| Filtro de várias colunas (F7) | ❌ não feito | busca em todas as colunas fica na caixa da grade |
| Abas 1–5 (Ctrl+1..5), aba anterior/próxima | ➕ melhorado | etapa 8; Ctrl+1..9, todos configuráveis |
| Restaurar dimensões dos painéis | ✅ feito | etapa 8 |

### Somente outros bancos

| Funcionalidade do HeidiSQL | Status | Onde/observação |
|---|---|---|
| SQLite (anexar, cifras), PostgreSQL (schemas, `COMMENT ON`), MSSQL, Interbase/Firebird | ❌ não feito | outros bancos, fora do escopo |
| ProxySQL Admin, kill via `mysql.rds_kill*` | ❌ não feito | variantes fora do escopo |

---

## Principais melhorias em relação ao HeidiSQL

- **Exportação de todas as linhas em streaming**: o HeidiSQL exporta só o que está carregado; aqui a consulta é
  reexecutada e as linhas vão direto para o arquivo, sem limite e sem truncar (etapa 5).
- **Dump com snapshot consistente** (`START TRANSACTION WITH CONSISTENT SNAPSHOT`) e cópia para outro database
  que corrige views e triggers qualificados com o database de origem (etapa 5).
- **Túnel SSH embutido**, sem plink/ssh nem porta local, com senha, chave, agente e **verificação de chave do
  servidor sempre ligada** (etapa 7).
- **Comparação de estruturas** entre databases (o "Synchronize" do HeidiSQL nunca ficou pronto), com ALTER que
  preserva dados e reaplicação conferida (etapa 6).
- **Gerenciador de usuários** com expiração de senha, bloqueio de conta, privilégios dinâmicos do MySQL 8, diff
  contra o servidor e senhas sempre mascaradas (etapa 6).
- **Busca no servidor por expressão regular**, com texto vinculado e curingas literais (etapa 6).
- **Substituição segura de rotinas e triggers**: valida com objeto temporário e restaura a versão anterior se
  a criação final falhar (etapa 3).
- **Edições com valores vinculados** (correto com `NO_BACKSLASH_ESCAPES`), gravadas em lote numa única transação,
  no máximo uma linha por instrução (etapa 2).
- **ALTER TABLE mínimo** calculado contra a definição atual do servidor, com teste de ida e volta sem ALTERs
  fantasmas; partições também alteradas e removidas (etapa 3).
- **Log de SQL completo** (wrapper do driver: nada escapa), com parâmetros, senhas mascaradas e filtros (etapa 1).
- **Avisos do servidor** exibidos junto do resultado de cada instrução, e EXPLAIN em vários formatos (etapa 4).
- **Políticas de segurança** em tudo que o app gera: perfis somente leitura, confirmação em produção e para
  operações destrutivas (todas as etapas).
- **Tarefas em segundo plano** canceláveis com progresso para dump, importação, execução de arquivo, busca e
  manutenção (etapas 5 e 6).
- **Exportação de conexões sem senhas** e segredos no keyring do sistema (etapa 7).
- **Atalhos configuráveis com detecção de conflitos** e palavras-chave em maiúsculas que respeitam strings e
  nomes (etapa 8).

## Não implementado

- **Outros bancos e variantes**: SQLite, PostgreSQL, MSSQL, Interbase/Firebird, ProxySQL Admin e RDS. O projeto
  é focado em MySQL/MariaDB.
- **Exclusivo do Windows**: named pipes, autenticação do Windows, biblioteca cliente (DLL) e JumpList. No Linux
  há socket Unix e o driver Go puro.
- **Serviços externos**: verificação de atualizações, estatísticas de uso, Sequal Suggest (IA) e formatadores
  online. Todos enviariam dados para terceiros.
- **Fora do escopo do desktop atual**: impressão, várias instâncias/nova janela, pacotes de idioma e de ícones,
  linha de comando no estilo do HeidiSQL e arquivo de configurações portátil.
- **Aparência configurável**: fontes da interface e da grade, cores do realce e por tipo de dado, linhas por
  registro, zoom com Ctrl+roda. Hoje tudo segue o tema.
- **Log em arquivo**: log de consultas DDL/DML por sessão, salvar ou gravar o log de SQL em arquivo.
- **Itens menores ainda abertos**: favoritos e tamanho na árvore, renomear database, `FLUSH TABLES WITH READ LOCK`,
  senha expirada no connect, tamanho de índices, UNION/INSERT_METHOD e COMPRESSED no editor de tabela,
  copiar/colar e arrastar colunas, duplicar linha com chaves, abrir URL, filtros pela área de transferência,
  diálogo "inserir arquivos" com marcadores, limite de resultados, envio do lote de uma vez (desligado por
  segurança), busca com condição livre e em rotinas, dump para a área de transferência e marcadores no nome do
  arquivo, `LOW_PRIORITY` na importação.
