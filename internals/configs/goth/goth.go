package configs

import (
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"
	"github.com/umohsamuel/impact/internals/configs/env"
)

func Goth(environmentVariable *env.EnvironmentVariables) {
	googleProvider := google.New(environmentVariable.OAuthProvider.Google.GoogleKey, environmentVariable.OAuthProvider.Google.GoogleSecret, environmentVariable.OAuthProvider.Google.GoogleCallback, "profile", "email")
	githubProvider := github.New(environmentVariable.OAuthProvider.Github.GithubKey, environmentVariable.OAuthProvider.Github.GithubSecret, environmentVariable.OAuthProvider.Github.GithubCallback, "user")

	goth.UseProviders(
		googleProvider,
		githubProvider,
	)
}
