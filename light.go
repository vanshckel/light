package main

import (
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/lightsail"
)

func main() {
	// AWS 访问密钥 ID 和密钥
	awsAccessKeyID := readInput("密钥: ")
	awsSecretAccessKey := readInput("秘密密钥: ")
	regionName := readInput("区域代码: ")
	number, _ := readInt("输入开机数量: ")

	// EC2 客户端，用于获取可用区信息
	sess := session.Must(session.NewSession(&aws.Config{
		Region:      aws.String(regionName),
		Credentials: credentials.NewStaticCredentials(awsAccessKeyID, awsSecretAccessKey, ""),
	}))

	ec2Client := ec2.New(sess)

	resp, err := ec2Client.DescribeAvailabilityZones(nil)
	if err != nil {
		fmt.Println("获取可用区失败:", err)
		return
	}

	availabilityZones := make([]string, 0)
	for _, zone := range resp.AvailabilityZones {
		availabilityZones = append(availabilityZones, aws.StringValue(zone.ZoneName))
	}

	availabilityZone := readInput("可用区域（输入空默认随机可用区域）：")
	var randomConf int
	if availabilityZone == "" {
		fmt.Println("使用默认配置，随机可用区")
		randomConf = 1
	}

	lightsailClient := lightsail.New(sess)

	// 创建实例，随机分配可用区
	blueprintID := "ubuntu_22_04" // 使用 Ubuntu 22.04 镜像
	bundleID := "nano_3_0"       // 2h0.5g 实例类型

	for i := 0; i < number; i++ {
		instanceName := fmt.Sprintf("lightsail-instance-%d", i+1)
		var az string
		if randomConf == 1 {
			az = availabilityZones[i%len(availabilityZones)]
		} else {
			az = availabilityZone
		}

		_, err := lightsailClient.CreateInstances(&lightsail.CreateInstancesInput{
			InstanceNames: aws.StringSlice([]string{instanceName}),
			AvailabilityZone: aws.String(az),
			BlueprintId:       aws.String(blueprintID),
			BundleId:          aws.String(bundleID),
			Tags: []*lightsail.Tag{
				{
					Key:   aws.String("Name"),
					Value: aws.String(instanceName),
				},
			},
		})
		if err != nil {
			fmt.Printf("创建实例 %s 失败: %v\n", instanceName, err)
			fmt.Printf("已经创建%d台机器\n", i)
			break
		}
		fmt.Printf("创建实例 %s 在 %s 成功\n", instanceName, az)
	}

	fmt.Println("等待实例启动中，20s后检查实例是否启动成功")
	time.Sleep(20 * time.Second)

	instances, err := getAllInstances(lightsailClient)
	if err != nil {
		fmt.Println("获取实例列表失败:", err)
		return
	}

	printInstances(instances)

	fmt.Printf("一共成功启动 %d 台机器\n", len(instances))

	fmt.Println("开始删除实例")

	for _, instance := range instances {
		_, err := lightsailClient.DeleteInstance(&lightsail.DeleteInstanceInput{
			InstanceName: instance.Name,
		})
		if err != nil {
			fmt.Printf("删除实例 %s 时出错: %v\n", aws.StringValue(instance.Name), err)
		} else {
			fmt.Printf("已成功删除实例 %s\n", aws.StringValue(instance.Name))
		}
	}

	fmt.Println("等待实例删除完成，20s后检查实例是否删除完成")
	time.Sleep(20 * time.Second)

	instances, err = getAllInstances(lightsailClient)
	if err != nil {
		fmt.Println("获取实例列表失败:", err)
		return
	}

	printInstances(instances)

	fmt.Printf("%s现有 %d 台机器\n", regionName, len(instances))

	fmt.Println("程序执行完毕，按任意键退出...")
	fmt.Scanln()
}

func getAllInstances(client *lightsail.Lightsail) ([]*lightsail.Instance, error) {
	instances := make([]*lightsail.Instance, 0)
	params := &lightsail.GetInstancesInput{}

	for {
		output, err := client.GetInstances(params)
		if err != nil {
			return nil, err
		}
		for _, instance := range output.Instances {
			instances = append(instances, instance)
		}

		if output.NextPageToken == nil {
			break
		}
		params.PageToken = output.NextPageToken
	}
	return instances, nil
}

func printInstances(instances []*lightsail.Instance) {
	for _, instance := range instances {
		publicIP := "N/A"
		if instance.PublicIpAddress != nil {
			publicIP = aws.StringValue(instance.PublicIpAddress)
		}
		fmt.Println(aws.StringValue(instance.Name), publicIP, aws.StringValue(instance.State.Name))
	}
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	var input string
	fmt.Scanln(&input)
	return input
}

func readInt(prompt string) (int, error) {
	fmt.Print(prompt)
	var number int
	_, err := fmt.Scanln(&number)
	return number, err
}
