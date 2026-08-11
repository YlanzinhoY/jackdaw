# ACBFR Updater

Instalador para Windows da atualização **1.0.6 Ubi update only** do Assassin's Creed Black Flag Resynced.

O aplicativo localiza a instalação na Steam, baixa o RAR diretamente para o disco e substitui os arquivos correspondentes na pasta do jogo. Nenhum executável é renomeado e nenhum backup individual é criado pelo instalador.

## Como funciona

- O download de `3.853.243.886 bytes` é transmitido com um buffer fixo de 1 MiB, sem carregar o RAR inteiro na memória.
- O tamanho final é validado antes da instalação.
- O RAR é inspecionado com o `tar.exe` nativo do Windows.
- Apenas arquivos comuns são aceitos; caminhos inseguros, links e entradas duplicadas são rejeitados.
- Os arquivos são extraídos em lotes de até 20.
- Cada lote é copiado para a pasta do jogo e removido da área temporária antes do próximo.
- Arquivos existentes, inclusive arquivos com atributo somente leitura, são substituídos.

Com aproximadamente 109 arquivos, a instalação usa seis lotes: cinco lotes de 20 e um lote final com os arquivos restantes.

## Requisitos

- Windows 10 ou Windows 11;
- Assassin's Creed Black Flag Resynced instalado pela Steam;
- conexão estável com a internet;
- permissão de escrita na pasta do jogo;
- espaço livre para o RAR de aproximadamente 3,6 GiB e para o maior lote extraído;
- `tar.exe`, incluído nas versões atuais do Windows.

## Como usar

1. Feche o jogo e a Steam.
2. Execute `acbfr.exe`.
3. Escolha **Procurar automaticamente na Steam** ou informe manualmente a pasta do jogo.
4. Aguarde o download terminar.
5. Aguarde todos os lotes serem extraídos e copiados.

Exemplo de caminho:

```text
D:\SteamLibrary\steamapps\common\Assassin's Creed Black Flag Resynced
```

Use as setas para navegar, `Enter` para confirmar, `Esc` para voltar e `Ctrl+C` para cancelar.

## Observações importantes

- A instalação substitui arquivos existentes diretamente.
- Não execute o jogo ou a Steam durante a atualização.
- Se um lote falhar depois que lotes anteriores foram copiados, a instalação pode ficar parcial. Nesse caso, execute o instalador novamente ou verifique os arquivos pela Steam antes de tentar outra vez.
- O RAR real de aproximadamente 3,6 GiB não faz parte dos testes automatizados nem do repositório.
- O link fornecido ao projeto possui assinatura com expiração em 12/08/2026 às 06:00 UTC. Um link renovado pode ser informado sem recompilar por meio da variável `ACBFR_DOWNLOAD_URL`.

Exemplo de link renovado:

```powershell
$env:ACBFR_DOWNLOAD_URL = "https://servidor/exemplo/update.rar"
.\acbfr.exe
```

## Desenvolvimento

Os testes usam um RAR pequeno e local que reproduz extração em lotes, subpastas e sobrescrita de arquivos sem baixar o pacote real.

```powershell
go mod download
go test ./...
go vet ./...
go build -o bin/acbfr.exe .
```

## Licença

Distribuído sob a [Apache License 2.0](LICENSE).

Este é um projeto independente e não possui afiliação com Ubisoft, Assassin's Creed ou Steam.
