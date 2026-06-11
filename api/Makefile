# Comandos de desarrollo del API.

# TEST_DATABASE_URL por defecto apunta al Postgres dockerizado (puerto 5544).
TEST_DATABASE_URL ?= postgres://focus:changeme@localhost:5544/focus365?sslmode=disable

# test corre la suite con -p 1 (un paquete a la vez). Los tests de integración
# comparten una sola DB de pruebas y cada paquete hace TRUNCATE users CASCADE;
# sin -p 1 los paquetes corren en paralelo y se borran los datos entre ellos.
.PHONY: test
test:
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) GOTOOLCHAIN=local go test -p 1 ./...

# vet revisa el código sin ejecutar tests.
.PHONY: vet
vet:
	GOTOOLCHAIN=local go vet ./...

# check corre vet + test, igual que la verificación previa a integrar.
.PHONY: check
check: vet test
