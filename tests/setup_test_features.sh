#!/bin/bash
rm -rf test_env
mkdir -p test_env
cd test_env

# Function to create a repo with its own remote
create_repo() {
    NAME=$1
    mkdir ${NAME}_remote.git
    cd ${NAME}_remote.git
    git init --bare
    git symbolic-ref HEAD refs/heads/master
    cd ..

    mkdir ${NAME}_init
    cd ${NAME}_init
    git init
    # Ensure we are on master
    git branch -m master
    git remote add origin ../${NAME}_remote.git
    touch README.md
    git add README.md
    git commit -m "Initial commit"
    git push -u origin master
    cd ..
    rm -rf ${NAME}_init

    git clone ./${NAME}_remote.git $NAME
}

# Repo 1: Parent Version Mismatch (should update)
create_repo repo_parent_mismatch
cd repo_parent_mismatch
cat <<EOF > pom.xml
<project>
  <parent>
    <groupId>com.example</groupId>
    <artifactId>parent</artifactId>
    <version>1.0.0</version>
  </parent>
  <artifactId>child</artifactId>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 2: Repositories Update (should update)
create_repo repo_repos
cd repo_repos
cat <<EOF > pom.xml
<project>
  <repositories>
    <repository>
      <id>gitlab-maven</id>
      <url>https://git.weka.de/api/v4/projects/592/packages/maven</url>
    </repository>
  </repositories>
</project>
EOF
git add pom.xml
git commit -m "Add pom.xml"
git push
cd ..

# Repo 3: CI Settings Update (should update)
create_repo repo_ci
cd repo_ci
cat <<'EOF' > ci-settings.xml
<settings>
  <servers>
    <server>
      <id>gitlab-maven</id>
      <configuration>
        <httpHeaders>
          <property>
            <name>Job-Token</name>
            <value>${CI_JOB_TOKEN}</value>
          </property>
        </httpHeaders>
      </configuration>
    </server>
  </servers>
</settings>
EOF
git add ci-settings.xml
git commit -m "Add ci-settings.xml"
git push
cd ..

echo "Test environment created in test_env with feature files"
