# Etapa 4 — Editor de consultas

**Status:** concluída. (Executar arquivo SQL grande sem abrir no editor ficou para a Etapa 5, junto da
importação, porque usa o mesmo executor em streaming.)

## Painel de ajuda (HeidiSQL: "query helpers")

Painel à direita do editor (botão ⓘ na barra da aba ou Ctrl+Shift+H), redimensionável e lembrado entre
sessões, com cinco seções:

| Seção | Conteúdo |
|---|---|
| Colunas | colunas da tabela escolhida (do database da aba); duplo clique insere o nome; geradores de SELECT/INSERT/UPDATE/DELETE com as colunas marcadas (ou todas) e WHERE pela chave |
| Ajuda SQL | a ajuda do **próprio servidor** (`HELP 'contents'`, categorias, tópicos com descrição e exemplo, busca com `%`); **F1** no editor abre o tópico da palavra sob o cursor (com busca por prefixo se não houver tópico exato) |
| Funções e palavras-chave | funções com assinatura e palavras reservadas do autocomplete; duplo clique insere |
| Snippets | arquivos `.sql` em `~/.config/anansi/snippets` (compatível com a ideia do HeidiSQL de snippets em pasta): inserir, criar a partir da seleção ou da aba, renomear, excluir, abrir a pasta |
| Parâmetros | parâmetros `:nome` encontrados no SQL da aba, com o valor de cada um |

A ajuda vem das tabelas `help_*` do servidor em vez da lista de funções embutida no HeidiSQL: está sempre na
versão certa do servidor e não traz o texto do HeidiSQL (GPL) para o projeto.

## Parâmetros de consulta

Como o "Bind parameters" do HeidiSQL: `:nome` fora de strings, identificadores, comentários e `:=` é
substituído antes da execução. Números, `NULL`, `0x…` e textos já entre aspas entram como digitados; o resto
vira texto entre aspas com escape. A substituição liga sozinha quando há parâmetros (pode ser desligada no
painel). Executando a instrução sob o cursor, só ela recebe a substituição, e os marcadores de erro continuam
na linha certa. Os valores ficam só na memória (não vão para o `workspace.json`).

## Execução

- **EXPLAIN** (Ctrl+Shift+E ou botão), com variantes no menu: `EXPLAIN FORMAT=JSON` e, conforme o servidor,
  `EXPLAIN FORMAT=TREE`/`EXPLAIN ANALYZE` (MySQL 8) ou `ANALYZE` (MariaDB). Respeita seleção e parâmetros.
- **Perfil**: botão que liga `SET SESSION profiling = 1` na conexão da aba, executa e acrescenta um resultado
  ⏱ com `SHOW PROFILE` (uma instrução) ou `SHOW PROFILES` (script). Nesse modo resultados grandes não são
  paginados (o perfil precisa da conexão livre).
- **Avisos do servidor**: depois de cada instrução (até 200 por script) o app lê `SHOW WARNINGS` e mostra os
  avisos sobre o resultado, na aba Mensagens e com ⚠ no seletor de resultados. O HeidiSQL só os escreve no
  log. Detalhe resolvido no caminho: no MariaDB instruções sem tabela (ex.: `SELECT 1`) não limpam a área de
  diagnóstico e "herdavam" os avisos da anterior; agora a aba limpa a área depois de ler avisos — exceto
  quando a próxima instrução é `SHOW WARNINGS`/`SHOW ERRORS`/`GET DIAGNOSTICS`, que continuam vendo os
  avisos. Pode ser desligado em Preferências.

## Resultados

- **Resultados editáveis** (HeidiSQL: "editable query results"): quando o resultado vem de um `SELECT` de uma
  só tabela (sem JOIN, GROUP BY, DISTINCT, UNION, subconsulta no FROM) e a tabela tem chave primária ou única
  NOT NULL presente no resultado, aparece o botão **Editar**. A grade editável da Etapa 2 é usada; colunas
  calculadas ou com alias diferente do nome ficam somente leitura; `*` respeita colunas INVISIBLE. Salvar usa
  a mesma transação com parâmetros da grade de dados e atualiza as linhas carregadas sem reexecutar a consulta.
  A análise fica em `sqllex.SingleTableSource` e a validação (tabela, chave, mapeamento de colunas) em
  `api.EditableResult`.
- **Ordenação local**: clicar no cabeçalho ordena as linhas já carregadas (crescente → decrescente → original),
  numérica para colunas numéricas; a consulta não é reexecutada.

## Abas

"Fechar abas à direita" e "Fechar todas" no menu da aba, além das ações que já existiam.

## Diferenças intencionais em relação ao HeidiSQL

- **"Send batch in one go"** não foi implementado: o app mantém `multiStatements` desativado por segurança e
  envia uma instrução por vez (o que também dá resultado, erro e aviso por instrução).
- Localizar/substituir usa o do Monaco (Ctrl+F / Ctrl+H, com regex, maiúsculas e palavra inteira).
- Formatação continua com `sql-formatter` local; o HeidiSQL oferece formatadores online, que enviariam o SQL
  para terceiros.

## Testes

- Unitários Go: `sqllex/source_test.go` (SELECTs editáveis e não editáveis: JOIN, vírgula, DISTINCT, GROUP BY,
  derivadas, UNION, FOR UPDATE; itens com alias, qualificados, calculados).
- Unitários frontend: `tests/params.test.ts` (detecção ignorando strings, identificadores, comentários, `:=` e
  `::`; literais; substituição de todas as ocorrências).
- Integração (`internal/integration/query_helpers_test.go`, MySQL e MariaDB): avisos na instrução certa (e não
  na seguinte, inclusive no MariaDB), `SHOW WARNINGS` do usuário ainda vê os avisos, perfil, resultados
  editáveis (`*`, colunas mistas, casos recusados), ajuda do servidor (conteúdo, tópico, inexistente) e
  snippets (inclusive recusa de `../`).
- No app: parâmetro `:maximo` substituído, edição do resultado salva, aviso 1292 exibido, F1 em `CONCAT`
  abrindo a ajuda, EXPLAIN.
