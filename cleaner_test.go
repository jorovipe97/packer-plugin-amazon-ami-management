package main

import (
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/mock/gomock"
)

func TestCleaner_RetrieveCandidateImages_KeepReleases(t *testing.T) {
	var (
		cfgs = []Config{
			{
				KeepReleases: 2,
				Tags: map[string]string{
					"Amazon_AMI_Management_Identifier": "packer-example",
				},
			},
			{
				KeepReleases: 2,
				Identifier:   "packer-example",
			},
		}
	)
	for _, cfg := range cfgs {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ec2mock := NewMockEC2API(ctrl)

		ec2mock.EXPECT().DescribeImages(&ec2.DescribeImagesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag:Amazon_AMI_Management_Identifier"),
					Values: []*string{
						aws.String("packer-example"),
					},
				},
			},
		}).Return(&ec2.DescribeImagesOutput{
			Images: []*ec2.Image{{
				ImageId:      aws.String("ami-12345a"),
				CreationDate: aws.String("2016-08-01T15:04:05.000Z"),
			}, {
				ImageId:      aws.String("ami-12345b"),
				CreationDate: aws.String("2016-08-04T15:04:05.000Z"),
			}, {
				ImageId:      aws.String("ami-12345c"),
				CreationDate: aws.String("2016-07-29T15:04:05.000Z"),
				BlockDeviceMappings: []*ec2.BlockDeviceMapping{{
					Ebs: &ec2.EbsBlockDevice{
						SnapshotId: aws.String("snap-12345a"),
					},
				}, {
					Ebs: &ec2.EbsBlockDevice{
						SnapshotId: aws.String("snap-12345b"),
					},
				}},
			}},
		}, nil)

		cleaner := &Cleaner{
			ec2conn: ec2mock,
			config:  cfg,
			now:     time.Now().UTC(),
		}

		images, err := cleaner.RetrieveCandidateImages()
		if err != nil {
			t.Fatalf("Unexpected error occurred: %s", err)
		}
		if len(images) != 1 {
			t.Fatalf("Unexpected image count: %d", len(images))
		}
		if *images[0].ImageId != "ami-12345c" {
			t.Fatalf("Unexpected image: %s", *images[0].ImageId)
		}
	}
}

func TestCleaner_RetrieveCandidateImages_KeepDays(t *testing.T) {
	var (
		cfgs = []Config{
			{
				KeepDays: 10,
				Tags: map[string]string{
					"Amazon_AMI_Management_Identifier": "packer-example",
				},
			},
			{
				KeepDays:   10,
				Identifier: "packer-example",
			},
		}
	)
	for _, cfg := range cfgs {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		ec2mock := NewMockEC2API(ctrl)

		ec2mock.EXPECT().DescribeImages(&ec2.DescribeImagesInput{
			Filters: []*ec2.Filter{
				{
					Name: aws.String("tag:Amazon_AMI_Management_Identifier"),
					Values: []*string{
						aws.String("packer-example"),
					},
				},
			},
		}).Return(&ec2.DescribeImagesOutput{
			Images: []*ec2.Image{{
				ImageId:      aws.String("ami-12345a"),
				CreationDate: aws.String("2016-08-01T15:04:05.000Z"),
			}, {
				ImageId:      aws.String("ami-12345b"),
				CreationDate: aws.String("2016-08-04T15:04:05.000Z"),
			}, {
				ImageId:      aws.String("ami-12345c"),
				CreationDate: aws.String("2016-07-29T15:04:05.000Z"),
				BlockDeviceMappings: []*ec2.BlockDeviceMapping{{
					Ebs: &ec2.EbsBlockDevice{
						SnapshotId: aws.String("snap-12345a"),
					},
				}, {
					Ebs: &ec2.EbsBlockDevice{
						SnapshotId: aws.String("snap-12345b"),
					},
				}},
			}},
		}, nil)

		cleaner := &Cleaner{
			ec2conn: ec2mock,
			config:  cfg,
			now:     time.Date(2016, time.August, 11, 11, 0, 0, 0, time.UTC),
		}

		images, err := cleaner.RetrieveCandidateImages()
		if err != nil {
			t.Fatalf("Unexpected error occurred: %s", err)
		}
		if len(images) != 1 {
			t.Fatalf("Unexpected image count: %d", len(images))
		}
		if *images[0].ImageId != "ami-12345c" {
			t.Fatalf("Unexpected image: %s", *images[0].ImageId)
		}
	}
}

func TestCleaner_DeleteImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ec2mock := NewMockEC2API(ctrl)

	ec2mock.EXPECT().DeregisterImage(&ec2.DeregisterImageInput{
		ImageId: aws.String("ami-12345c"),
		DryRun:  aws.Bool(false),
	}).Return(&ec2.DeregisterImageOutput{}, nil)
	ec2mock.EXPECT().DeleteSnapshot(&ec2.DeleteSnapshotInput{
		SnapshotId: aws.String("snap-12345a"),
		DryRun:     aws.Bool(false),
	}).Return(&ec2.DeleteSnapshotOutput{}, nil)
	ec2mock.EXPECT().DeleteSnapshot(&ec2.DeleteSnapshotInput{
		SnapshotId: aws.String("snap-12345b"),
		DryRun:     aws.Bool(false),
	}).Return(&ec2.DeleteSnapshotOutput{}, nil)

	cleaner := &Cleaner{
		ec2conn: ec2mock,
	}

	err := cleaner.DeleteImage(&ec2.Image{
		ImageId:      aws.String("ami-12345c"),
		CreationDate: aws.String("2016-07-29T15:04:05.000Z"),
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{{
			Ebs: &ec2.EbsBlockDevice{
				SnapshotId: aws.String("snap-12345a"),
			},
		}, {
			Ebs: &ec2.EbsBlockDevice{
				SnapshotId: aws.String("snap-12345b"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("Unexpected error occurred: %s", err)
	}
}

func TestCleaner_DeleteImage_EphemeralDevise(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ec2mock := NewMockEC2API(ctrl)

	ec2mock.EXPECT().DeregisterImage(&ec2.DeregisterImageInput{
		ImageId: aws.String("ami-12345c"),
		DryRun:  aws.Bool(false),
	}).Return(&ec2.DeregisterImageOutput{}, nil)
	ec2mock.EXPECT().DeleteSnapshot(&ec2.DeleteSnapshotInput{
		SnapshotId: aws.String("snap-12345a"),
		DryRun:     aws.Bool(false),
	}).Return(&ec2.DeleteSnapshotOutput{}, nil)

	cleaner := &Cleaner{
		ec2conn: ec2mock,
	}

	err := cleaner.DeleteImage(&ec2.Image{
		ImageId:      aws.String("ami-12345c"),
		CreationDate: aws.String("2016-07-29T15:04:05.000Z"),
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{{
			Ebs: &ec2.EbsBlockDevice{
				SnapshotId: aws.String("snap-12345a"),
			},
		}, {
			Ebs: nil,
		}, {
			Ebs: nil,
		}},
	})
	if err != nil {
		t.Fatalf("Unexpected error occurred: %s", err)
	}
}

func TestCleaner_DeleteImage_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ec2mock := NewMockEC2API(ctrl)

	ec2mock.EXPECT().DeregisterImage(&ec2.DeregisterImageInput{
		ImageId: aws.String("ami-12345a"),
		DryRun:  aws.Bool(true),
	}).Return(nil, awserr.New(
		"DryRunOperation",
		"Request would have succeeded, but DryRun flag is set.",
		errors.New("Request would have succeeded, but DryRun flag is set."),
	))
	ec2mock.EXPECT().DeleteSnapshot(&ec2.DeleteSnapshotInput{
		SnapshotId: aws.String("snap-12345a"),
		DryRun:     aws.Bool(true),
	}).Return(nil, awserr.New(
		"DryRunOperation",
		"Request would have succeeded, but DryRun flag is set.",
		errors.New("Request would have succeeded, but DryRun flag is set."),
	))

	cleaner := &Cleaner{
		ec2conn: ec2mock,
		config: Config{
			DryRun: true,
		},
	}

	err := cleaner.DeleteImage(&ec2.Image{
		ImageId:      aws.String("ami-12345a"),
		CreationDate: aws.String("2016-07-29T15:04:05.000Z"),
		BlockDeviceMappings: []*ec2.BlockDeviceMapping{{
			Ebs: &ec2.EbsBlockDevice{
				SnapshotId: aws.String("snap-12345a"),
			},
		}},
	})
	if err != nil {
		t.Fatalf("Unexpected error occurred: %s", err)
	}
}

func TestCleaner_setLaunchTemplateUsed_ResolveAliases(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	ec2mock := NewMockEC2API(ctrl)

	ec2mock.EXPECT().DescribeLaunchTemplates(&ec2.DescribeLaunchTemplatesInput{}).Return(&ec2.DescribeLaunchTemplatesOutput{
		LaunchTemplates: []*ec2.LaunchTemplate{
			{
				LaunchTemplateId: aws.String("lt-12345a"),
			},
		},
	}, nil)
	ec2mock.EXPECT().DescribeLaunchTemplateVersions(&ec2.DescribeLaunchTemplateVersionsInput{
		LaunchTemplateId: aws.String("lt-12345a"),
	}).Return(&ec2.DescribeLaunchTemplateVersionsOutput{
		LaunchTemplateVersions: []*ec2.LaunchTemplateVersion{
			{
				LaunchTemplateName: aws.String("cool-service-launch-template"),
				VersionNumber:      aws.Int64(3),
				LaunchTemplateData: &ec2.ResponseLaunchTemplateData{
					ImageId: aws.String("resolve:ssm:/acme/cool-service/latest-ami"),
				},
			},
			{
				// This version should not generate an extra request.
				LaunchTemplateName: aws.String("cool-service-launch-template"),
				VersionNumber:      aws.Int64(2),
				LaunchTemplateData: &ec2.ResponseLaunchTemplateData{
					ImageId: aws.String("resolve:ssm:/acme/cool-service/latest-ami"),
				},
			},
			{
				LaunchTemplateName: aws.String("cool-service-launch-template"),
				VersionNumber:      aws.Int64(1),
				LaunchTemplateData: &ec2.ResponseLaunchTemplateData{
					ImageId: aws.String("ami-12345a"),
				},
			},
		},
	}, nil)

	// We only expect an additional call to DescribeLaunchTemplateVersions since version 2 and 3
	// have the same value in SSM Parameter "/acme/cool-service/latest-ami".
	ec2mock.EXPECT().DescribeLaunchTemplateVersions(&ec2.DescribeLaunchTemplateVersionsInput{
		LaunchTemplateId: aws.String("lt-12345a"),
		Versions:         []*string{aws.String("3")},
		ResolveAlias:     aws.Bool(true),
	}).Return(&ec2.DescribeLaunchTemplateVersionsOutput{
		LaunchTemplateVersions: []*ec2.LaunchTemplateVersion{
			{
				VersionNumber: aws.Int64(3),
				LaunchTemplateData: &ec2.ResponseLaunchTemplateData{
					ImageId: aws.String("ami-12345b"), // This is the value in SSM Paramter "/acme/cool-service/latest-ami"
				},
			},
		},
	}, nil)

	cleaner := &Cleaner{
		ec2conn: ec2mock,
		config: Config{
			ResolveAliases: true,
		},
		used:            map[string]*Used{},
		resolvedAliases: map[string]string{},
	}

	err := cleaner.setLaunchTemplateUsed()

	if err != nil {
		t.Fatalf("Unexpected error occurred: %s", err)
	}

	if len(cleaner.used) != 2 {
		t.Fatalf("Unexpected used count: %d", len(cleaner.used))
	}

	if len(cleaner.resolvedAliases) != 1 {
		t.Fatalf("Unexpected resolvedAliases count: %d", len(cleaner.resolvedAliases))
	}
}
