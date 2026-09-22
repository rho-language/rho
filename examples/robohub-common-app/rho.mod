module robohub-common-app
version 0.1.0
main = "src/main.rho"

dep robohub-common {
    type = "c"
    version = "1.0.0"
    path = "/home/miraur/projects/robohub-common"
    include = "include"
    src = "src"
}